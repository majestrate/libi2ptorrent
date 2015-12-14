package libi2ptorrent

import (
  i2p "github.com/majestrate/i2p-tools/sam3"
  "encoding/json"
  "io/ioutil"
  "path/filepath"
)

type Config struct {
  RootDirectory string
  NumSeeders    int
  I2P           i2p.Config
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
  NumSeeders: 10,
  I2P: i2p.Config{
    Addr: "127.0.0.1:7656",
    Session: "bulkseeder",
    Keyfile: "",
    Opts: map[string]string{
      "inbound.quantity" : "8",
      "outbound.quantity" : "8",
    },
  },
}
