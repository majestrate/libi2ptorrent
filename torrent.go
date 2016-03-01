package libi2ptorrent

import (
	"bytes"
	"encoding/base32"
	"fmt"
	"github.com/majestrate/i2p-tools/lib/i2p"
	"github.com/majestrate/libi2ptorrent/bitfield"
	"github.com/majestrate/libi2ptorrent/filestore"
	"github.com/majestrate/libi2ptorrent/metainfo"
	"github.com/majestrate/libi2ptorrent/tracker"
	"github.com/op/go-logging"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	Stopped = iota
	Leeching
	Seeding
)

var logger = logging.MustGetLogger("libtorrent")

type Torrent struct {
	sam              i2p.StreamSession
	listener         *Listener
	meta             *metainfo.Metainfo
	fileStore        *filestore.FileStore
	config           *Config
	bitf             *bitfield.Bitfield
	swarm            []*peer
	incomingPeer     chan *peer
	incomingPeerAddr chan i2p.I2PDestHash
	swarmTally       swarmTally
	readChan         chan peerDouble
	trackers         []*tracker.Tracker
	state            int
	stateLock        sync.Mutex
	peerID           []byte
}

func NewTorrent(m *metainfo.Metainfo, config *Config, bitf *bitfield.Bitfield) (tor *Torrent, err error) {
	tor = &Torrent{
		peerID:           []byte(fmt.Sprintf("li2pt-%d%d%d%d", rand.Int63(), rand.Int63(), rand.Int63(), rand.Int63()))[:20],
		config:           config,
		meta:             m,
		incomingPeer:     make(chan *peer, 100),
		incomingPeerAddr: make(chan i2p.I2PDestHash, 100),
		readChan:         make(chan peerDouble, 50),
		state:            Stopped,
	}
	// Extract file information to create a slice of torrentStorers
	tfiles := make([]filestore.TorrentStorer, 0)
	var tfile filestore.TorrentStorer
	for _, file := range tor.meta.Files {
		if tfile, err = filestore.NewTorrentFile(tor.config.RootDirectory, file.Path, file.Length); err != nil {
			logger.Error("Failed to create file %s: %s", file.Path, err)
			return
		}
		tfiles = append(tfiles, tfile)
	}

	// Now we can create our filestore.
	if tor.fileStore, err = filestore.NewFileStore(tfiles, tor.meta.Pieces, tor.meta.PieceLength); err != nil {
		logger.Error("Failed to create filestore: %s", err)
		return
	}

	// given from external validation
	tor.bitf = bitf

	return
}

func (tor *Torrent) Connect() (err error) {

	cfg := tor.config.I2P
	session := strings.Trim(base32.StdEncoding.EncodeToString(tor.meta.InfoHash), "=")
	session += strings.ToLower(session[:10])
	session += fmt.Sprintf("-%d", rand.Int63())
	logger.Info("connecting to i2p with session %s", session)
	tor.sam, err = i2p.NewSessionEasy(cfg.Addr, "")
	if err == nil {
		if err == nil {
			tor.listener = NewListener(tor.sam)
			tor.listener.AddTorrent(tor)
		}
	}
	return
}

func (tor *Torrent) Validate() (bitf *bitfield.Bitfield, err error) {
	tor.bitf, err = tor.fileStore.Validate()
	bitf = tor.bitf
	return
}

func (tor *Torrent) Close() {
	if tor.listener != nil {
		tor.listener.Close()
		tor.listener = nil
	}
	if tor.sam != nil {
		tor.sam.Close()
		tor.sam = nil
	}
}

func (tor *Torrent) Start() {
	var err error
	for {
		err = tor.Connect()
		if err == nil {
			break
		} else {
			// failed to connect
			logger.Error("Failed to connect to i2p router: %s", err)
			tor.Close()
			time.Sleep(time.Second)
		}
	}
	logger.Info("Torrent starting: %s", tor.meta.Name)

	// Set initial state
	tor.stateLock.Lock()
	if tor.bitf.SumTrue() == tor.bitf.Length() {
		tor.state = Seeding
	} else {
		tor.state = Leeching
	}
	tor.stateLock.Unlock()

	// start listener
	go func() {
		err := tor.listener.Listen()
		if err != nil {
			logger.Error("failed to listen: %s", err.Error())
			tor.Close()
		}
	}()

	// Create trackers
	for _, tkr := range tor.meta.AnnounceList {
		tkr, err := tracker.NewTracker(tor.sam, tkr, tor, tor.incomingPeerAddr)
		if err != nil {
			logger.Error("Failed to create tracker: %s", err)
			continue
		}
		tor.trackers = append(tor.trackers, tkr)
		tkr.Start()
	}

	// Tracker loop
	go func() {
		for {
			peerAddr := <-tor.incomingPeerAddr
			// Only attempt to connect to other peers whilst leeching
			//if tor.state != Leeching {
			//  continue
			//}
			logger.Info("connecting out to %s", peerAddr)
			go func() {
				conn, err := tor.sam.Dial("tcp", peerAddr.String()+":0")
				if err == nil {
					tor.AddPeer(conn, nil)
				} else {
					logger.Debug("Failed to connect to tracker peer address %s: %s", peerAddr, err)
				}
			}()
		}
	}()

	// Peer loop
	go func() {
		for {
			select {
			case peer := <-tor.incomingPeer:
				// Add to swarm slice
				logger.Debug("Connected to new peer: %s", peer.name)
				tor.swarm = append(tor.swarm, peer)
			case <-time.After(time.Second * 5):
				// Unchoke interested peers
				// TODO: Implement maximum unchoked peers
				// TODO: Implement optimistic unchoking algorithm
				for _, peer := range tor.swarm {
					if peer.GetPeerInterested() && peer.GetAmChoking() {
						logger.Debug("Unchoking peer %s", peer.name)
						peer.write <- &unchokeMessage{}
						peer.SetAmChoking(false)
					}
				}
			}
		}
	}()

	// Receive loop
	go func() {
		for {
			peerDouble := <-tor.readChan
			peer := peerDouble.peer
			msg := peerDouble.msg
			if peer == nil {
				continue
			}
			if msg == nil {
				continue
			}
			switch msg := msg.(type) {
			case *chokeMessage:
				logger.Debug("Peer %s has choked us", peer.name)
				peer.SetPeerChoking(true)
			case *unchokeMessage:
				logger.Debug("Peer %s has unchoked us", peer.name)
				peer.SetPeerChoking(false)
			case *interestedMessage:
				logger.Debug("Peer %s has said it is interested", peer.name)
				peer.SetPeerInterested(true)
			//case *uninterestedMessage:
			//	logger.Debug("Peer %s has said it is uninterested", peer.name)
			case *haveMessage:
				pieceIndex := int(msg.pieceIndex)
				logger.Debug("Peer %s has piece %d", peer.name, pieceIndex)
				if pieceIndex >= tor.meta.PieceCount {
					logger.Debug("Peer %s sent an out of range have message")
					// TODO: Shutdown client
					peer.Close()
				}
				peer.HasPiece(pieceIndex)
				// TODO: Update swarmTally
			case *bitfieldMessage:
				logger.Debug("Peer %s has sent us its bitfield", peer.name)
				// Raw parsed bitfield has no actual length. Let's try to set it.
				if err := msg.bitf.SetLength(tor.meta.PieceCount); err != nil {
					logger.Error(err.Error())
					// TODO: Shutdown client
					peer.Close()
					break
				}
				peer.SetBitfield(msg.bitf)
				tor.swarmTally.AddBitfield(msg.bitf)
			case *requestMessage:
				if peer.GetAmChoking() || !tor.bitf.Get(int(msg.pieceIndex)) {
					logger.Debug("Peer %s has asked for a block (%d, %d, %d), but we are rejecting them", peer.name, msg.pieceIndex, msg.blockOffset, msg.blockLength)
					// Add naughty points
					break
				}
				logger.Debug("Peer %s has asked for a block (%d, %d, %d), going to fetch block", peer.name, msg.pieceIndex, msg.blockOffset, msg.blockLength)
				block, err := tor.fileStore.GetBlock(int(msg.pieceIndex), int64(msg.blockOffset), int64(msg.blockLength))
				if err != nil {
					logger.Error(err.Error())
					peer.Close()
					break
				}
				logger.Debug("Peer %s has asked for a block (%d, %d, %d), sending it to them", peer.name, msg.pieceIndex, msg.blockOffset, msg.blockLength)
				peer.write <- &pieceMessage{
					pieceIndex:  msg.pieceIndex,
					blockOffset: msg.blockOffset,
					data:        block,
				}
			case *pieceMessage:
				logger.Debug("Piece message from %s", peer.name)
				break
			//case *cancelMessage:
			//  logger.Debug("cancel message from %s", peer.name)
			default:
				logger.Debug("Peer %s sent unknown message: %s", peer.name, msg)
			}
		}
	}()
}

func (t *Torrent) String() string {
	s := `Torrent: %x
    Name: '%s'
    Piece length: %d
    Announce lists: %v`
	return fmt.Sprintf(s, t.meta.InfoHash, t.meta.Name, t.meta.PieceLength, t.meta.AnnounceList)
}

func (t *Torrent) InfoHash() []byte {
	return t.meta.InfoHash
}

func (t *Torrent) State() (state int) {
	t.stateLock.Lock()
	state = t.state
	t.stateLock.Unlock()
	return
}

func (t *Torrent) AddPeer(conn net.Conn, hs *handshake) {
	// Set 60 second limit to connection attempt
	conn.SetDeadline(time.Now().Add(time.Minute))

	// Send handshake
	if err := newHandshake(t.InfoHash(), t.PeerId()).BinaryDump(conn); err != nil {
		logger.Debug("%s Failed to send handshake to connection: %s", conn.RemoteAddr(), err)
		conn.Close()
		return
	}

	// If hs is nil, this means we've attempted to establish the connection and need to wait
	// for their handshake in response
	var err error
	if hs == nil {
		if hs, err = parseHandshake(conn); err != nil {
			logger.Debug("%s Failed to parse incoming handshake: %s", conn.RemoteAddr(), err)
			conn.Close()
			return
		} else if !bytes.Equal(hs.infoHash, t.InfoHash()) {
			logger.Debug("%s Infohash did not match for connection", conn.RemoteAddr())
			conn.Close()
			return
		}
	}

	peer := newPeer(string(hs.peerId), conn, t.readChan)
	if peer.write != nil {
		peer.write <- &bitfieldMessage{bitf: t.bitf}
		t.incomingPeer <- peer
	}

	conn.SetDeadline(time.Time{})
}

func (t *Torrent) Downloaded() int64 {
	// TODO:
	return 0
}

func (t *Torrent) Uploaded() int64 {
	// TODO:
	return 0
}

func (t *Torrent) Left() int64 {
	// TODO:
	return 0
}

func (t *Torrent) PeerId() []byte {
	return t.peerID
}
