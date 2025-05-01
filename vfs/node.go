// Package vfs implements a virtual filesystem that stores IPFS CIDs in a  tree-like structure.
package vfs

import (
	"context"
	"log"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

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
		return 0
	}

	out.Mode = fuse.S_IFREG | 0444
	out.Nlink = 1
	out.Mtime = uint64(n.obj.ModTime().Unix())
	out.Ctime = out.Mtime
	out.Atime = out.Mtime
	out.Size = n.obj.Size()
	const bs = 512
	out.Blksize = bs
	out.Blocks = (out.Size + bs - 1) / bs
	return 0
}
