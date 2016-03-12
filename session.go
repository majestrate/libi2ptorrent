package libi2ptorrent

import (
	"github.com/majestrate/i2p-tools/lib/i2p"
)

// a libi2ptorrent session holding many torrents and 1 listener
type Session struct {
	listener *Listener
	sam      i2p.StreamSession
	config   *Config
}

func (sess *Session) Connect() (err error) {
	sess.sam, err = i2p.NewStreamSessionEasy(sess.config.I2P.Addr, "")
	if err == nil {
		sess.listener = NewListener(sess.sam)
	}
	return
}

func (sess *Session) Run() {
	sess.listener.Listen()
}

func (sess *Session) Close() {

	if sess.listener != nil {
		sess.listener.Close()
		sess.listener = nil
	}
	if sess.sam != nil {
		sess.sam.Close()
		sess.sam = nil
	}
}

func NewSession(conf *Config) *Session {
	return &Session{
		config: conf,
	}
}
