# libi2ptorrent

i2p torrent library in go

under development

## bittorrent client

seed only bittorrent client:

building:

    go build -u github.com/majestrate/libi2ptorrent/cmd/seeder

running:

By default `seeder` looks in the current working directory for torrent files and their associated contents in `.`

`seeder` will seed all torrents with 6 peers in parallel by default

to run `seeder` with non default settings you can give it a config file: `seeder -config /path/to/config.json`

see `examples/seeder-example.json` for example configuration
