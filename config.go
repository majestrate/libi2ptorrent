package libi2ptorrent

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
)

type I2PConfig struct {
	Addr string
	Keyfile string
}

type Config struct {
	RootDirectory string
	NumSeeders    int
	I2P           I2PConfig
}

func (cfg *Config) ListTorrents() (torrentFiles []string, err error) {
	return filepath.Glob(filepath.Join(cfg.RootDirectory, "*.torrent"))
}

func LoadConfig(fname string) (cfg *Config, err error) {
	var data []byte
	data, err = ioutil.ReadFile(fname)
	if err == nil {
		cfg = new(Config)
		err = json.Unmarshal(data, cfg)
	}
	return
}

var DefaultConfig = &Config{
	RootDirectory: ".",
	NumSeeders:    1,
	I2P: I2PConfig{
		Addr:    "127.0.0.1:7656",
		Keyfile: "",
	},
}
