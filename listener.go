package libi2ptorrent

import (
  "fmt"
  "net"
)

type Listener struct {
  torrents map[string]*Torrent
  listener net.Listener
}

func NewListener(listener net.Listener) (l *Listener) {
  l = &Listener{
    listener: listener,
    torrents: make(map[string]*Torrent),
  }
  return
}

func (l *Listener) AddTorrent(tor *Torrent) {
  infoHash := fmt.Sprintf("%x", tor.InfoHash())
  l.torrents[infoHash] = tor
}

func (l *Listener) Listen() (err error) {

  // Begin accepting incoming peers
  
  for {
    var conn net.Conn
    conn, err = l.listener.Accept()
    if err != nil {
      logger.Error("Listener unexpectedly quit: %s", err)
      return
    }
    
    go func() {
      hs, err := parseHandshake(conn)
      if err != nil {
        logger.Error("%s Initial handshake failed: %s", conn.RemoteAddr(), err)
        conn.Close()
        return
      }
      
      infoHash := fmt.Sprintf("%x", hs.infoHash)
      if tor, ok := l.torrents[infoHash]; ok {
        logger.Debug("%s Incoming peer connection: %s", conn.RemoteAddr(), hs.peerId)
        tor.AddPeer(conn, hs)
      } else {
        logger.Info("%s Incoming peer connection using expired/invalid infohash", conn.RemoteAddr())
        conn.Close()
      }
      return
    }()
  }
  return
}

func (l *Listener) Close() error {
  return l.listener.Close()
}
