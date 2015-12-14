package libi2ptorrent

import (
	"github.com/majestrate/libi2ptorrent/bitfield"
	"io"
	"sync"
  "time"
	//"testing/iotest"
)

type peer struct {
	name           string
	conn           io.ReadWriteCloser
	write          chan binaryDumper
	read           chan peerDouble
	amChoking      bool
	amInterested   bool
	peerChoking    bool
	peerInterested bool
	mutex          sync.RWMutex
	bitf           *bitfield.Bitfield
}

type peerDouble struct {
	msg  interface{}
	peer *peer
}

func newPeer(name string, conn io.ReadWriteCloser, readChan chan peerDouble) (p *peer) {
	p = &peer{
		name:           name,
		conn:           conn,
		write:          make(chan binaryDumper, 10),
		read:           readChan,
		amChoking:      true,
		amInterested:   false,
		peerChoking:    true,
		peerInterested: false,
	}

	// Write loop
	go func() {
		for {
			//conn := iotest.NewWriteLogger("Writing", conn)
			// TODO: send regular keep alive requests
      if p.write == nil {
        return
      }
			msg, ok := <-p.write
      if ok {
        if err := msg.BinaryDump(conn); err != nil {
          // TODO: Close peer
          logger.Error("%s Received error writing to connection: %s", p.name, err)
          p.Close()
          return
        }
      } else {
        p.Close()
        return
      }
		}
	}()

	// Read loop
	go func() {
		for {
			//conn := iotest.NewReadLogger("Reading", conn)
			msg, err := parsePeerMessage(conn)

			if _, ok := err.(unknownMessage); ok {
				// Log unknown messages and then ignore
				logger.Info(err.Error())
			} else if err != nil {
				logger.Debug("%s Received error reading connection: %s", p.name, err)
        p.Close()
        return
			} else if p.read != nil {
        p.read <- peerDouble{msg: msg, peer: p}
      }
		}
	}()

	return
}

func (p *peer) GetAmChoking() (b bool) {
	p.mutex.RLock()
	b = p.amChoking
	p.mutex.RUnlock()
	return
}

func (p *peer) SetAmChoking(b bool) {
	p.mutex.Lock()
	p.amChoking = b
	p.mutex.Unlock()
}

func (p *peer) SetPeerChoking(b bool) {
	p.mutex.Lock()
	p.peerChoking = b
	p.mutex.Unlock()
}

func (p *peer) GetPeerInterested() (b bool) {
	p.mutex.RLock()
	b = p.peerInterested
	p.mutex.RUnlock()
	return
}

func (p *peer) SetPeerInterested(b bool) {
	p.mutex.Lock()
	p.peerInterested = b
	p.mutex.Unlock()
}

func (p *peer) SetBitfield(bitf *bitfield.Bitfield) {
	p.mutex.Lock()
	p.bitf = bitf
	p.mutex.Unlock()
}

func (p *peer) HasPiece(index int) {
	p.mutex.Lock()
	p.bitf.SetTrue(index)
	p.mutex.Unlock()
}

func (p *peer) Close() {
  time.Sleep(1000)
  if p.write != nil {
    c := p.write
    p.write = nil
    close(c)
  }
  if p.read != nil {
    c := p.read
    p.read = nil
    close(c)
  }
  p.conn.Close()
}
