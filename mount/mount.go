package mount

// Mount implements a FUSE filesystem that exposes a VFS.
// It allows users to interact with the repository as if it were a regular filesystem.

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("mount")

const MountTimeout = time.Second * 5

// Mount represents a filesystem mount.
type Mount interface {
	// MountPoint is the path at which this mount is mounted
	MountPoint() string

	// Unmounts the mount
	Unmount() error

	// WaitDone
	WaitDone() error
}

type mount struct {
	mountpoint string
	server     *fuse.Server
	done       chan bool
}

func (m *mount) MountPoint() string {
	return m.mountpoint
}

func (m *mount) Unmount() error {
	log.Debugf("Unmount(%s)", m.mountpoint)
	if m.server == nil {
		return fmt.Errorf("not mounted")
	}
	return m.server.Unmount()
}

func (m *mount) WaitDone() error {
	select {
	case <-m.done:
		return nil
	case <-time.After(MountTimeout):
		return fmt.Errorf("timeout waiting for mount to close")
	}
}

// Mount mounts the repository at the given mountpoint.
func NewMount(ctx context.Context, vfs fs.InodeEmbedder, mountpoint string) (Mount, error) {
	var err error

	// Create the mountpoint directory if it doesn't exist.
	if _, err = os.Stat(mountpoint); os.IsNotExist(err) {
		if err = os.MkdirAll(mountpoint, 0755); err != nil {
			return nil, fmt.Errorf("failed to create mountpoint: %w", err)
		}
	}

	// Create a new FUSE server.
	server, err := fs.Mount(mountpoint, vfs, &fs.Options{
		MountOptions: fuse.MountOptions{
			AllowOther: true,
			Debug:      false,
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to mount filesystem: %w", err)
	}

	m := &mount{
		mountpoint: mountpoint,
		server:     server,
		done:       make(chan bool),
	}

	// Start a function that would trigger the unmount
	go func() {
		<-ctx.Done()
		log.Infof("Context cancelled, unmounting...")
		if err := m.Unmount(); err != nil {
			log.Errorf("Failed to unmount: %v", err)
		}
		m.done <- true
	}()

	return m, nil
}
