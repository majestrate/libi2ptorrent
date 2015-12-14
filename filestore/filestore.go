package filestore

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"github.com/majestrate/libi2ptorrent/bitfield"
	"io"
	"os"
	"path/filepath"
)

type FileStore struct {
	tfiles      []TorrentStorer
	hashes      [][]byte
	pieceLength int64
	totalLength int64
}

func NewFileStore(tfiles []TorrentStorer, hashes [][]byte, pieceLength int64) (fs *FileStore, err error) {
	fs = &FileStore{
		tfiles:      tfiles,
		hashes:      hashes,
		pieceLength: pieceLength,
	}

	for _, tfile := range tfiles {
		fs.totalLength += tfile.Length()
	}

	return
}

func (fs *FileStore) Validate() (bitf *bitfield.Bitfield, err error) {
	bitf = bitfield.NewBitfield(len(fs.hashes))

	for i, _ := range fs.hashes {
		var ok bool
		ok, err = fs.validatePiece(i)
		if err != nil {
			return
		} else if ok {
			bitf.SetTrue(i)
		}
	}
	return
}

func (fs *FileStore) validatePiece(index int) (ok bool, err error) {
	block, err := fs.GetBlock(index, 0, fs.getPieceLength(index))
	if err != nil {
		return
	}

	h := sha1.New()
	h.Write(block)
	if bytes.Equal(h.Sum(nil), fs.hashes[index]) {
		ok = true
	}
	return
}

func (fs *FileStore) getPieceLength(index int) int64 {
	if index == len(fs.hashes)-1 {
		return fs.totalLength % fs.pieceLength
	} else {
		return fs.pieceLength
	}
}

func (fs *FileStore) GetBlock(pieceIndex int, offset int64, length int64) (block []byte, err error) {
	if length+offset > fs.getPieceLength(pieceIndex) {
		err = errors.New("Requested block overran piece length")
		return
	}

	block = make([]byte, length)
	segment := block

	offset = int64(pieceIndex)*fs.pieceLength + offset

	for _, tfile := range fs.tfiles {
		var lengthRead int
		lengthRead, err = tfile.ReadAt(segment, offset)

		if err == nil {
			// We've read it all!
			break
		} else if err == io.EOF {
			// We haven't read anything, or only a partial read
			segment = segment[lengthRead:]
			if offset-tfile.Length() < 0 {
				offset = 0
			} else {
				offset -= tfile.Length()
			}
		} else if err != nil {
			// Something else went wrong
			break
		}
	}

	return
}

type TorrentStorer interface {
	io.ReaderAt
	Length() int64
}

type TorrentFile struct {
	lth  int64
	path string
	fd   *os.File
}

func NewTorrentFile(rootDirectory string, path string, length int64) (tfile *TorrentFile, err error) {
	if len(path) == 0 {
		err = errors.New("Path must have at least 1 component.")
		return
	}

	// Root directory must already exist
	rootDirectoryFileInfo, err := os.Stat(rootDirectory)
	if err != nil {
		return
	}
	if !rootDirectoryFileInfo.IsDir() {
		err = errors.New(rootDirectory + " is not a directory")
		return
	}

	absPath := filepath.Join(rootDirectory, path)

	// Create any required parent directories
	dirs := filepath.Dir(absPath)
	if err = os.MkdirAll(dirs, 0755); err != nil {
		return
	}

	// Create or open file
	fd, err := os.OpenFile(absPath, os.O_RDWR|os.O_CREATE, 0644)

	// Stat for size of file
	stat, err := fd.Stat()
	if err != nil {
		return
	}
	if length-stat.Size() < 0 {
		err = errors.New("File already exists and is larger than expected size. Aborting.")
		return
	}

	// Now pad the file from the end until it matches required size
	err = fd.Truncate(length)
	if err != nil {
		return
	}

	tfile = &TorrentFile{
		path: path,
		lth:  length,
		fd:   fd,
	}

	return
}

func (tf *TorrentFile) ReadAt(p []byte, off int64) (n int, err error) {
	n, err = tf.fd.ReadAt(p, off)
	return
}

func (tf *TorrentFile) Length() int64 {
	return tf.lth
}

func (tf *TorrentFile) String() string {
	return fmt.Sprintf("[File: %s Length: %dbytes]", tf.path, tf.lth)
}
