package ipfs

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/ipfs/boxo/files"
)

type fileHandle struct {
	mu   sync.Mutex
	obj  *Object
	file files.File
}

func (fh *fileHandle) String() string {
	return fh.obj.String()
}

func (fh *fileHandle) Close(ctx context.Context) error {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	log.Debugf("ipfs.FileHandle.Close(%s)", fh)
	return fh.file.Close()
}

func (fh *fileHandle) Read(ctx context.Context, dest []byte, off int64) (int, error) {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	log.Debugf("ipfs.FileHandle.Read(%s): %d bytes at offset %d", fh.obj.cid, len(dest), off)

	_, err := fh.file.Seek(off, io.SeekStart)
	if err != nil {
		log.Errorf("ipfs.FileHandle.Read(%s): Seek error [%v]", fh.obj.cid, err)
		return 0, fmt.Errorf("Read error [%w]", err)
	}

	n, err := fh.file.Read(dest)
	if err == io.EOF {
		log.Debugf("ipfs.FileHandle.Read(%s): EOF", fh.obj.cid)
		err = nil
	}
	if err != nil {
		log.Errorf("ipfs.FileHandle.Read(%s): Read error [%v]", fh.obj.cid, err)
		return 0, fmt.Errorf("Read error [%w]", err)
	}

	return n, nil
}

func (fh *fileHandle) Size(ctx context.Context) (int64, error) {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	log.Debugf("ipfs.FileHandle.Size(%s)", fh)
	return fh.file.Size()
}
