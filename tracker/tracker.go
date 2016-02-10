package tracker

import (
	"errors"
	"fmt"
	"github.com/majestrate/i2p-tools/lib/i2p"
	"github.com/op/go-logging"
	"github.com/zeebo/bencode"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

var NONE = ""
var STARTED = "started"
var STOPPED = "stopped"
var COMPLETED = "completed"

var logger = logging.MustGetLogger("libtorrent")

type TorrentStatter interface {
	InfoHash() []byte
	Downloaded() int64
	Uploaded() int64
	Left() int64
	PeerId() []byte
}

// function that is used to dial out
type DialFunc func(net, addr string) (net.Conn, error)

type Tracker struct {
	sam          i2p.Session
	transport    *http.Transport
	url          *url.URL
	stat         TorrentStatter
	n            uint // This is used like a tcp backoff mechanism
	nextAnnounce time.Duration
	stop         chan struct{}
	peerChan     chan i2p.I2PDestHash
	announce     chan struct{} // Used to force an announce
}

func NewTracker(s i2p.Session, address string, stat TorrentStatter, peerChan chan i2p.I2PDestHash) (trk *Tracker, err error) {
	// Verify valid http / or udp address
	url, err := url.Parse(address)
	if err != nil {
		return
	} else if url.Scheme != "http" {
		err = errors.New(fmt.Sprintf("newTracker: unknown scheme '%s'", url.Scheme))
		return
	}

	trk = &Tracker{
		sam: s,
		transport: &http.Transport{
			Dial: s.Dial,
		},
		url:      url,
		stat:     stat,
		peerChan: peerChan,
		stop:     make(chan struct{}),
	}
	return
}

// extract i2p desthashes from compact response
func extractPeers(peers []byte) (addrs []i2p.I2PDestHash, err error) {
	l := len(peers)
	if l > 0 {
		// there are peers
		if l%32 == 0 {
			// length is correct
			idx := 0
			// extract destinations
			for idx < l {
				var dest i2p.I2PDestHash
				copy(dest[:], peers[idx:idx+32])
				addrs = append(addrs, dest)
				idx += 32
			}
		} else {
			// bad length
			err = errors.New("peers bytestring is not multiple of 32 bytes")
		}
	}
	return
}

func (tkr *Tracker) Start() {
	tkr.nextAnnounce = 0
	event := STARTED

	go func() {
	L:
		for {
			select {
			case <-time.After(tkr.nextAnnounce):
				// Time to announce
			case <-tkr.announce:
				// We've been forced to announce
			case <-tkr.stop:
				break L
			}

			annReq := &Request{
				Addr:       tkr.sam.Addr(),
				InfoHash:   tkr.stat.InfoHash(),
				PeerId:     tkr.stat.PeerId(),
				Downloaded: tkr.stat.Downloaded(),
				Left:       tkr.stat.Left(),
				Uploaded:   tkr.stat.Uploaded(),
				Event:      event,
				NumWant:    50,
			}
			annRes, err := tkr.httpAnnounce(annReq)
			if err != nil {
				logger.Info("Failed to contact tracker %s, error: %s", tkr.url, err)
				// Attempt again using a backoff pattern 60*2^n
				tkr.nextAnnounce = time.Second * 60 * time.Duration(1<<tkr.n)
				tkr.n++
				continue
			}
			peers, err := extractPeers(annRes.Peers)
			if err == nil {
				// Success!
				logger.Info("Got %d peers from tracker %s. Next announce in %d seconds", len(peers), tkr.url, annRes.Interval)
				if annRes.Interval == 0 {
					annRes.Interval = 60
				}
				tkr.nextAnnounce = time.Second * time.Duration(annRes.Interval)
				event = NONE
				for _, peer := range peers {
					tkr.peerChan <- peer
				}
			} else {
				logger.Error("invalid response from tracker %s: %s", tkr.url, err.Error())
			}
		}

		// Announce STOPPED
		annReq := &Request{
			Addr:       tkr.sam.Addr(),
			InfoHash:   tkr.stat.InfoHash(),
			PeerId:     tkr.stat.PeerId(),
			Downloaded: tkr.stat.Downloaded(),
			Left:       tkr.stat.Left(),
			Uploaded:   tkr.stat.Uploaded(),
			Event:      STOPPED,
			NumWant:    50,
		}
		// Ignore failure, we're only making a 'best effort' to shutdown cleanly
		tkr.httpAnnounce(annReq)
	}()
}

func (tkr *Tracker) Stop() {
	close(tkr.stop)
}

func (tkr *Tracker) Announce() {
	go func() { tkr.announce <- struct{}{} }()
}

func (tkr *Tracker) httpAnnounce(req *Request) (resp *Response, err error) {
	cl := &http.Client{
		Transport: tkr.transport,
	}
	var r *http.Response
	r, err = cl.Get(req.buildURL(tkr.url))
	if err == nil {
		resp, err = readAnnounceResponse(r.Body)
	}
	r.Body.Close()
	return
}

type Request struct {
	InfoHash   []byte
	PeerId     []byte
	Downloaded int64
	Left       int64
	Uploaded   int64
	Event      string
	Addr       net.Addr
	NumWant    int32
}

// build request url to use in http request
func (req *Request) buildURL(u *url.URL) (str string) {
	val := make(url.Values)
	val.Add("compact", "1")
	val.Add("info_hash", string(req.InfoHash))
	val.Add("peer_id", string(req.PeerId))
	val.Add("event", req.Event)
	val.Add("uploaded", fmt.Sprintf("%d", req.Uploaded))
	val.Add("downloaded", fmt.Sprintf("%d", req.Downloaded))
	val.Add("numwant", fmt.Sprintf("%d", req.NumWant))
	val.Add("ip", req.Addr.String()+".i2p")
	val.Add("port", "6881")
	val.Add("left", fmt.Sprintf("%d", req.Left))
	return u.String() + "?" + val.Encode()
}

type Response struct {
	Action      string `bencode:"action"`
	Interval    int32  `bencode:"interval"`
	MinInterval int32  `bencode:"min interval"`
	Leechers    int32  `bencode:"incomplete"`
	Seeders     int32  `bencode:"complete"`
	Peers       []byte `bencode:"peers"`
}

// read announce response from reader
func readAnnounceResponse(r io.Reader) (resp *Response, err error) {
	resp = new(Response)
	dec := bencode.NewDecoder(r)
	err = dec.Decode(resp)
	return
}
