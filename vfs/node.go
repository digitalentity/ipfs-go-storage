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

type VFSObject interface {
	GetAttr(ctx context.Context) (*VFSObjectAttr, error)
	Open(ctx context.Context) error
	Read(ctx context.Context, dest []byte, off int64) (int, error)
}

type VFSObjectNode struct {
	fs.Inode
	root *VFSRoot
	obj  VFSObject
}

func (n *VFSObjectNode) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	log.Printf("VFSObjectNode.Getattr: %v", n)
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
	log.Printf("VFSObjectNode.Open: %v", n)

	if n.obj == nil {
		return nil, 0, syscall.EACCES
	}

	err := n.obj.Open(ctx)
	if err != nil {
		log.Printf("VFSObject Open failed: %v", err)
		return nil, 0, syscall.EIO
	}

	// We don't return a filehandle since we don't really need one.
	// The file content is immutable, so hint the kernel to cache the data.
	return nil, fuse.FOPEN_KEEP_CACHE, 0
}

func (n *VFSObjectNode) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	log.Printf("VFSObjectNode.Read: %v", n)
	if n.obj == nil {
		return nil, syscall.EACCES
	}

	cnt, err := n.obj.Read(ctx, dest, off)
	if err != nil {
		log.Printf("VFSObject Read failed: %v", err)
		return nil, syscall.EIO
	}

	return fuse.ReadResultData(dest[:cnt]), 0
}
