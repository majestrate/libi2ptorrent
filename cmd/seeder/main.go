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
	var err error
	var cfg *libtorrent.Config
	if cfg_fname == "" {
		cfg = libtorrent.DefaultConfig
	} else {
		cfg, err = libtorrent.LoadConfig(cfg_fname)
		if err != nil {
			logger.Error("failed to load config: %s", err.Error())
			return
		}
	}
	logger.Info("starting up bulk seeder")
	ls, err := cfg.ListTorrents()
	if err != nil {
		logger.Error("cannot read torrent list: %s", err.Error())
		return
	}
	logger.Info("loading %d torrents", len(ls))
	var metas []*metainfo.Metainfo
	for _, fname := range ls {
		f, err := os.Open(fname)
		if err == nil {
			meta, err := metainfo.ParseMetainfo(f)
			f.Close()
			if err == nil {
				logger.Info("loaded torrent %s", meta.Name)
				metas = append(metas, meta)
			} else {
				logger.Info("failed to load torrent file %s", fname)
			}
		}
	}
	for _, meta := range metas {
		go func(num int, meta *metainfo.Metainfo) {
			logger.Info("checking %s", meta.Name)
			t, err := libtorrent.NewTorrent(meta, cfg, nil)
			if err == nil {
				bitf, err := t.Validate()
				if err == nil {
					t.Start()
					// start more
					for num > 0 {
						// spawn more
						t, err = libtorrent.NewTorrent(meta, cfg, bitf)
						if err == nil {
							logger.Info("starting worker %d for %s", num, meta.Name)
							go func() {
								t.Start()
							}()
						} else {
							logger.Error("Failed to start worker number %d: %s", num, err.Error())
						}
						num--
					}
				}
			} else {
				logger.Error("failed to start torrent %s: %s", meta.Name, err.Error())
			}
		}(cfg.NumSeeders, meta)
	}
	for {
		time.Sleep(time.Second)
	}
}
