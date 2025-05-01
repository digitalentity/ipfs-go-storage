// Package vfs implements a virtual filesystem that stores IPFS CIDs in a  tree-like structure.
package vfs

import (
	"context"
	"log"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type VFSObjectAttr struct {
	Size    uint64    // Size of the object in bytes
	ModTime time.Time // Modification time of the object
}

type VFSObjectHandle interface {
	Close(ctx context.Context) error
	Read(ctx context.Context, dest []byte, off int64) (int, error)
	Size(ctx context.Context) (int64, error)
}

type VFSObject interface {
	GetAttr(ctx context.Context) (*VFSObjectAttr, error)
	Open(ctx context.Context) (VFSObjectHandle, error)
}

type VFSObjectHandleImpl struct {
	fh VFSObjectHandle
}

var _ = (fs.FileReleaser)((*VFSObjectHandleImpl)(nil))
var _ = (fs.FileReader)((*VFSObjectHandleImpl)(nil))

func (fh *VFSObjectHandleImpl) Release(ctx context.Context) syscall.Errno {
	log.Printf("VFSObjectHandleImpl.Release: %s", fh)
	fh.fh.Close(ctx)
	return 0
}

func (fh *VFSObjectHandleImpl) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	log.Printf("VFSObjectHandleImpl.Read: %s", fh)
	cnt, err := fh.fh.Read(ctx, dest, off)
	if err != nil {
		log.Printf("VFSObject Read failed: %v", err)
		return nil, syscall.EIO
	}
	return fuse.ReadResultData(dest[:cnt]), 0
}

type VFSObjectNode struct {
	fs.Inode
	root *VFSRoot
	obj  VFSObject
}

var _ = (fs.NodeGetattrer)((*VFSObjectNode)(nil))
var _ = (fs.NodeOpener)((*VFSObjectNode)(nil))
var _ = (fs.NodeReleaser)((*VFSObjectNode)(nil))

func (n *VFSObjectNode) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	log.Printf("VFSObjectNode.Getattr: %s", n)
	if n.obj == nil {
		out.Mode = 0000
		out.Nlink = 1
		return syscall.EACCES
	}

	attr, err := n.obj.GetAttr(ctx)
	if err != nil {
		log.Printf("VFSObject GetAttr failed: %v", err)
		out.Mode = 0000
		out.Nlink = 1
		return syscall.EACCES
	}

	out.Mode = fuse.S_IFREG | 0444
	out.Nlink = 1
	out.Mtime = uint64(attr.ModTime.Unix())
	out.Ctime = out.Mtime
	out.Atime = out.Mtime
	out.Size = attr.Size
	const bs = 512
	out.Blksize = bs
	out.Blocks = (attr.Size + bs - 1) / bs
	return 0
}

func (n *VFSObjectNode) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	log.Printf("VFSObjectNode.Open: %s", n)

	if n.obj == nil {
		return nil, 0, syscall.EACCES
	}

	fh, err := n.obj.Open(ctx)
	if err != nil {
		log.Printf("VFSObject Open failed: %v", err)
		return nil, 0, syscall.EIO
	}

	// We don't return a filehandle since we don't really need one.
	// The file content is immutable, so hint the kernel to cache the data.
	return &VFSObjectHandleImpl{fh: fh}, fuse.FOPEN_KEEP_CACHE, 0
}

func (n *VFSObjectNode) Release(ctx context.Context, fh fs.FileHandle) syscall.Errno {
	log.Printf("VFSObjectNode.Release: %s", n)
	return fh.(*VFSObjectHandleImpl).Release(ctx)
}

func (n *VFSObjectNode) Flush(ctx context.Context, fh fs.FileHandle) syscall.Errno {
	log.Printf("VFSObjectNode.Flush: %s", n)
	return 0
}

// func (n *VFSObjectNode) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
// 	log.Printf("VFSObjectNode.Read: %v", n)
// 	if n.obj == nil {
// 		return nil, syscall.EACCES
// 	}

// 	cnt, err := n.obj.Read(ctx, dest, off)
// 	if err != nil {
// 		log.Printf("VFSObject Read failed: %v", err)
// 		return nil, syscall.EIO
// 	}

// 	return fuse.ReadResultData(dest[:cnt]), 0
// }
