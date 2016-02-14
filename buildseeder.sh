#!/usr/bin/env bash
set -e
root=$(readlink -e $(dirname $0))
cd $root
export GOPATH=$root/go
mkdir -p $GOPATH
go get -v -u github.com/majestrate/libi2ptorrent/cmd/seeder
cp -a $GOPATH/bin/seeder $root
