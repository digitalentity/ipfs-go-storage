package commands

import (
	"context"
	"ipfs-go-storage/config"
	"ipfs-go-storage/ipfs"
	"ipfs-go-storage/ipfs/objectset"
	"ipfs-go-storage/mount"
	"ipfs-go-storage/vfs"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const ObjectSetWatcherInterval = 60 * time.Second

func RunMount(ctx context.Context, cfg *config.Config, mountPoint string) {
	cctx, cancel := context.WithCancel(ctx)

	log.Infof("Mounting ipfs-go-storage...")

	// Create IPFS connector
	connector, err := ipfs.NewConnector(cfg)
	if err != nil {
		log.Fatal(err)
	}

	// Start IPFS
	if err := connector.Start(cctx); err != nil {
		log.Fatal(err)
	}

	osw := objectset.NewWatcher(cfg, connector)
	if err := osw.Start(cctx); err != nil {
		log.Fatal(err)
	}

	// Wait until we receive an ObjectSet or have a timeout. This will block
	var o *objectset.ObjectSet

	log.Infof("Waiting for initial ObjectSet...")
	select {
	case o = <-osw.Recv():
		// All good, we can continue now
	case <-time.After(ipfs.IPFSObjectSetTimeout):
		log.Fatalf("Timeout waiting for initial ObjectSet")
	}

	// Create the VFS
	fs := vfs.NewVFSRoot(o.Objects)

	// Mount the filesystem
	mount, err := mount.NewMount(cctx, fs, mountPoint)
	if err != nil {
		log.Fatal(err)
	}

	log.Infof("Mounted at %s", mount.MountPoint())

	fs.Print()

	// Start a goroutine to receive updates from the ObjectSetWatcher
	go func() {
		for {
			select {
			case o := <-osw.Recv():
				if o == nil {
					log.Infof("ObjectSetWatcher closed")
					return
				}
				fs.UpdateObjectSet(cctx, o.Objects)
				fs.Print()
			case <-cctx.Done():
				return
			}
		}
	}()

	// Wait for a signal to shut down.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// Cancel the context
	cancel()

	// Wait for ObjectSetWatcher to shut down
	if err := osw.WaitDone(); err != nil {
		log.Errorf("Error waiting for ObjectSetWatcher: %v", err)
	}

	// Wait for unmount
	if err := mount.WaitDone(); err != nil {
		log.Errorf("Error waiting for unmount: %v", err)
	}

	// Wait for IPFS connector shutdown
	if err := connector.WaitDone(); err != nil {
		log.Errorf("Error waiting for IPFS connector shutdown: %v", err)
	}

	// Wait for ObjectSetWatcher shutdown
	if err := osw.WaitDone(); err != nil {
		log.Errorf("Error stopping ObjectSetWatcher: %v", err)
	}
}
