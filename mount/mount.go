package mount

// Mount implements a FUSE filesystem that exposes a VFS.
// It allows users to interact with the repository as if it were a regular filesystem.

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

var (
	MountTimeout = time.Second * 5
)

// Mount represents a filesystem mount.
type Mount interface {
	// MountPoint is the path at which this mount is mounted
	MountPoint() string

	// Unmounts the mount
	Unmount() error
}

type mount struct {
	mountpoint string
	server     *fuse.Server
}

func (m *mount) MountPoint() string {
	return m.mountpoint
}

func (m *mount) Unmount() error {
	if m.server == nil {
		return fmt.Errorf("not mounted")
	}
	return m.server.Unmount()
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
	}

	// Handle signals for graceful unmounting.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case sig := <-sigChan:
			log.Printf("Received signal %v, unmounting...", sig)
			if err := m.Unmount(); err != nil {
				log.Printf("Failed to unmount: %v", err)
			}
			os.Exit(0)
		case <-ctx.Done():
			if err := m.Unmount(); err != nil {
				log.Printf("Failed to unmount: %v", err)
			}
		}
	}()

	return m, nil
}
