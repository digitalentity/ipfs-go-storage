package commands

import (
	"context"
	"ipfs-go-storage/config"
	"ipfs-go-storage/ipfs"
	"ipfs-go-storage/mount"
	"ipfs-go-storage/vfs"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const ObjectSetWatcherInterval = 60 * time.Second

func RetreiveObjectSet(ctx context.Context, connector *ipfs.Connector) (map[string]vfs.VFSObject, error) {
	objectset := make(map[string]vfs.VFSObject)

	data := map[string]string{
		"/Anime/Kusuriya no Hitorigoto TV-2 01.mkv": "QmRR2wi98aHLfGf8Nu5MxM33BTrChyaQ9phNCHH2RF78WC",
	}

	for path, id := range data {
		obj, err := ipfs.NewIPFSObject(connector, id)
		if err != nil {
			return nil, err
		}

		objectset[path] = obj
	}

	return objectset, nil
}

func updateObjectSet(ctx context.Context, connector *ipfs.Connector, vfs *vfs.VFSRoot) error {
	_, err := RetreiveObjectSet(ctx, connector)
	if err != nil {
		return err
	}

	// vfs.Update(objectset)
	return nil
}

// objectSetWather retreives the IPNS path and CID it points to periodically
// If the new ObjectSet is newer than the currently mounted, the VFS is updated.
func objectSetWather(ctx context.Context, connector *ipfs.Connector, vfs *vfs.VFSRoot) {
	// Ticker to periodically listen to updates
	t := time.NewTicker(ObjectSetWatcherInterval)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			// Retreive the VFS ObjectSet from the IPNS/IPFS storage
			err := updateObjectSet(ctx, connector, vfs)
			if err != nil {
				log.Printf("Error updating objectset: %v", err)
			}
		case <-ctx.Done():
			log.Printf("Context cancelled, stopping objectset watcher...")
			return
		}
	}
}

func RunMount(ctx context.Context, cfg *config.Config, mountPoint string, p2pPort int) {
	cctx, cancel := context.WithCancel(ctx)

	log.Println("Mounting ipfs-go-storage...")

	// Create IPFS connector
	connector, err := ipfs.NewConnector(cfg)
	if err != nil {
		log.Fatal(err)
	}

	// Start IPFS
	if err := connector.Start(cctx); err != nil {
		log.Fatal(err)
	}

	// Retreive the VFS ObjectSet from the IPNS/IPFS storage
	objectset, err := RetreiveObjectSet(cctx, connector)
	if err != nil {
		log.Fatal(err)
	}

	// Create the VFS
	fs := vfs.NewVFSRoot(objectset)

	// Mount the filesystem
	mount, err := mount.NewMount(cctx, fs, mountPoint)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Mounted at %s", mount.MountPoint())

	fs.Print()

	// Start a watcher to update VFS
	go objectSetWather(cctx, connector, fs)

	// Wait for a signal to shut down.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// Cancel the context
	cancel()

	// Wait for unmount
	if err := mount.WaitDone(); err != nil {
		log.Printf("Error waiting for unmount: %v", err)
	}

	// Wait for IPFS connector shutdown
	if err := connector.WaitDone(); err != nil {
		log.Printf("Error waiting for IPFS connector shutdown: %v", err)
	}
}
