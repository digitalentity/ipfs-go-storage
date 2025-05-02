package ipfs

import (
	"context"
	"fmt"
	"ipfs-go-storage/vfs"
	"sync"
	"time"

	"github.com/ipfs/boxo/files"
	"github.com/ipfs/go-cid"
)

type Object struct {
	// Connector to the IPFS BitSwap service
	ipfs *Connector

	// Object CID and VFS metadata
	cid cid.Cid

	// Run-time object state
	mu        sync.Mutex
	attrValid bool              // Attributes fetch was successful. VFS is read-only, so this needs to happen only once.
	attr      vfs.VFSObjectAttr // Cached attributes

	lastError    error     // Cached error
	errorTimeout time.Time // Error timeout
}

func NewObject(ipfs *Connector, id string, mt time.Time) (*Object, error) {
	c, err := cid.Parse(id)
	if err != nil {
		return nil, err
	}

	return &Object{
		ipfs:      ipfs,
		cid:       c,
		attr:      vfs.VFSObjectAttr{ModTime: mt},
		attrValid: false,
	}, nil
}

func (o *Object) open(ctx context.Context) (*fileHandle, error) {
	log.Debugf("ipfs.Object.open(%s)", o)

	// Return the last error if it did not expire yet.
	if o.lastError != nil && o.errorTimeout.After(time.Now()) {
		return nil, o.lastError
	}

	uf, err := o.ipfs.GetUnixfile(ctx, o.cid)
	if err != nil {
		log.Errorf("ipfs.Object.open(%s): %v", o, err)
		o.lastError = fmt.Errorf("Open error [%w]", err)
		o.errorTimeout = time.Now().Add(IPFSObjectErrorTimeout)
		return nil, o.lastError
	}

	fh := &fileHandle{
		mu:   sync.Mutex{},
		obj:  o,
		file: uf.(files.File),
	}

	return fh, nil
}

func (o *Object) fetchAttributes(ctx context.Context) error {
	if o.attrValid {
		return nil
	}

	fh, err := o.open(ctx)
	if err != nil {
		return err
	}

	size, err := fh.Size(ctx)
	if err != nil {
		return err
	}

	o.attr.Size = uint64(size)
	o.attrValid = true

	fh.Close(ctx)
	return nil
}

func (o *Object) String() string {
	return o.cid.String()
}

func (o *Object) GetAttr(ctx context.Context) (*vfs.VFSObjectAttr, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.fetchAttributes(ctx); err != nil {
		return nil, err
	}
	return &o.attr, nil
}

func (o *Object) Open(ctx context.Context) (vfs.VFSObjectHandle, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	log.Infof("ipfs.Object.Open(%s)", o)
	return o.open(ctx)
}
