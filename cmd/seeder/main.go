package main

//
// i2p bittorrent bulk seeder
//

import (
	libtorrent "github.com/majestrate/libi2ptorrent"
	"github.com/majestrate/libi2ptorrent/metainfo"
	"github.com/op/go-logging"

	"flag"
	"os"
	"time"
)

var logger = logging.MustGetLogger("seeder")
var cfg_fname string

func init() {
	flag.StringVar(&cfg_fname, "config", "", "path to config file")
}

func main() {
	flag.Parse()
	logging.SetLevel(logging.INFO, "torrent")
	var err error
	var cfg *libtorrent.Config
	if cfg_fname == "" {
		cfg = libtorrent.DefaultConfig
	} else {
		cfg, err = libtorrent.LoadConfig(cfg_fname)
		if err != nil {
			logger.Errorf("failed to load config: %s", err.Error())
			return
		}
	}
	if cfg.Debug {
		logging.SetLevel(logging.DEBUG, "torrent")
	}
	logger.Info("starting up bulk seeder")
	ls, err := cfg.ListTorrents()
	if err != nil {
		logger.Errorf("cannot read torrent list: %s", err.Error())
		return
	}
	logger.Infof("loading %d torrents", len(ls))
	var metas []*metainfo.Metainfo
	for _, fname := range ls {
		f, err := os.Open(fname)
		if err == nil {
			meta, err := metainfo.ParseMetainfo(f)
			f.Close()
			if err == nil {
				logger.Infof("loaded torrent %s", meta.Name)
				metas = append(metas, meta)
			} else {
				logger.Infof("failed to load torrent file %s", fname)
			}
		}
	}
	var sessions []*libtorrent.Session
	// open N sessions
	num := cfg.NumSeeders
	for num > 0 {
		logger.Infof("Creating session %d", num)
		s := libtorrent.NewSession(cfg)
		err = s.Connect()
		if err != nil {
			logger.Errorf("Error creating session %d: %s", num, err.Error())
			return
		}
		sessions = append(sessions, s)
		num--
	}

	for _, meta := range metas {
		go func(meta *metainfo.Metainfo, sessions []*libtorrent.Session) {
			logger.Infof("checking %s", meta.Name)
			t, err := libtorrent.NewTorrent(meta, cfg, nil)
			if err == nil {
				_, err := t.Validate()
				if err == nil {
					logger.Infof("check success for %s", meta.Name)
					// attach this torrent to all sessions
					for _, sess := range sessions {
						t.Attach(sess)
					}
					t.Start()
				} else {
					logger.Errorf("failed to valid data for torrent %s: %s", meta.Name, err.Error())
				}
			} else {
				logger.Errorf("failed to start torrent %s: %s", meta.Name, err.Error())
			}
		}(meta, sessions)
	}
	for {
		time.Sleep(time.Second)
	}
}
