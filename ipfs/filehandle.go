package ipfs

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/ipfs/boxo/files"
)

type FileHandle struct {
	obj  *Object
	mu   sync.Mutex
	file files.File
}

func (fh *FileHandle) String() string {
	return fh.obj.String()
}

func (fh *FileHandle) Close(ctx context.Context) error {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	log.Printf("ipfs.FileHandle.Close(%s)", fh)
	return fh.file.Close()
}

func (fh *FileHandle) Read(ctx context.Context, dest []byte, off int64) (int, error) {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	log.Printf("ipfs.FileHandle.Read(%s)", fh)

	_, err := fh.file.Seek(off, io.SeekStart)
	if err != nil {
		return 0, fmt.Errorf("Read error [%w]", err)
	}
	n, err := fh.file.Read(dest)
	if err == io.EOF {
		err = nil
	}
	return n, err
}

func (fh *FileHandle) Size(ctx context.Context) (int64, error) {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	log.Printf("ipfs.FileHandle.Size(%s)", fh)
	return fh.file.Size()
}
