// Package vfs implements a virtual filesystem that stores IPFS CIDs in a  tree-like structure.
package vfs

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// No actual file data is stored in the VFS.

type VFSObject interface {
	Size() uint64
	ModTime() time.Time
}

type VFSRoot struct {
	fs.Inode
	mutex     sync.Mutex
	objectset map[string]VFSObject // Map from a path to an object (for files)
}

func NewVFSRoot(objectset map[string]VFSObject) *VFSRoot {
	root := &VFSRoot{}
	root.objectset = objectset
	return root
}

// UpdateObjectSet updates the VFS tree with a new ObjectSet.
// Objects that exist in the VFS but not in the new ObjectSet are pruned and their Inodes are recursively removed.
// Objects that don't exist in the VFS are created.
func (r *VFSRoot) UpdateObjectSet(ctx context.Context, objectset map[string]VFSObject) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Prune objects that no longer exist in the new objectset
	for path, _ := range r.objectset {
		if _, ok := objectset[path]; !ok {
			r.removeObject(ctx, path)
		}
	}

	// Add or update objects from the new objectset
	for path, obj := range objectset {
		r.addObject(ctx, path, obj)
	}

	// Finally update the objectset map
	r.objectset = objectset
}

func (r *VFSRoot) removeObject(ctx context.Context, path string) {
	dir, base := filepath.Split(path)

	// Look up the path
	p := &r.Inode
	for _, part := range strings.Split(dir, string(os.PathSeparator)) {
		if part == "" {
			continue
		}

		ch := p.GetChild(part)
		if ch == nil {
			// The directory doesn't exist in the VFS, so the object doesn't exist either.
			return
		}
		p = ch
	}

	// Look up the object itself
	ch := p.GetChild(base)
	if ch == nil {
		// The object doesn't exist in the VFS.
		return
	}

	// Remove the object's inode from the VFS tree.
	p.RmChild(base)

	// Clean up empty directories in the VFS tree.
	p = &r.Inode
	for _, part := range strings.Split(dir, string(os.PathSeparator)) {
		if part == "" {
			continue
		}

		parent := p
		p = p.GetChild(part)
		if p == nil {
			break
		}

		if len(p.Children()) == 0 {
			parent.RmChild(part)
		}
	}
}

func (r *VFSRoot) addObject(ctx context.Context, path string, obj VFSObject) {
	dir, base := filepath.Split(path)

	p := &r.Inode
	for _, part := range strings.Split(dir, string(os.PathSeparator)) {
		if part == "" {
			continue
		}

		ch := p.GetChild(part)
		if ch == nil {
			ch = p.NewPersistentInode(ctx, &fs.Inode{}, fs.StableAttr{Mode: fuse.S_IFDIR | 0555})
			p.AddChild(part, ch, true)
		}
		p = ch
	}
	ch := p.NewPersistentInode(ctx, &VFSObjectNode{root: r, obj: obj}, fs.StableAttr{Mode: fuse.S_IFREG | 0444})
	p.AddChild(base, ch, true)
}

func (r *VFSRoot) OnAdd(ctx context.Context) {
	log.Printf("VFSRoot.OnAdd")
	r.mutex.Lock()
	defer r.mutex.Unlock()

	for path, obj := range r.objectset {
		r.addObject(ctx, path, obj)
	}
}

func (r *VFSRoot) Print() {
	log.Printf("VFSRoot.Print")
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Walk the Inode tree and print the hierarchy
	var printInode func(inode *fs.Inode, indent string)
	printInode = func(inode *fs.Inode, indent string) {
		if inode == &r.Inode {
			log.Printf("%s/", indent)
		} else {
			name, _ := inode.Parent()
			log.Printf("%s- %s (%s)", indent, name, inode.StableAttr())
		}

		for _, child := range inode.Children() {
			printInode(child, indent+"  ")
		}
	}

	printInode(&r.Inode, "")
}
