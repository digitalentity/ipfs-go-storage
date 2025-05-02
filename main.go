package main

import (
	"context"
	"flag"
	"ipfs-go-storage/ipfs"
	"ipfs-go-storage/mount"
	"ipfs-go-storage/vfs"
	"log"
	"os"
	"os/signal"
	"syscall"
)

var (
	ipfsPeer   = flag.String("bitswap-peer", "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWPzN6y3VHiWVTSqf4R3yuEWjtaZhjDPZhiYH7q1bBiGVi", "IPFS peer address to connect to")
	p2pPort    = flag.Int("port", 4002, "Port for libP2P host")
	mountPoint = flag.String("mountpoint", "", "Path to mount the filesystem at")
)

func BuildObjectSet(ctx context.Context, connector *ipfs.Connector) (map[string]vfs.VFSObject, error) {
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

// main is the entry point of the application.
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Parse command-line options
	flag.Parse()
	log.Println("Starting ipfs-go-storage...")

	// Create IPFS connector
	connector, err := ipfs.NewConnector(*ipfsPeer, *p2pPort)
	if err != nil {
		log.Fatal(err)
	}

	// Start IPFS
	if err := connector.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer connector.Close()

	// Build the VFS ObjectSet
	objectset, err := BuildObjectSet(ctx, connector)
	if err != nil {
		log.Fatal(err)
	}

	// Create the VFS
	fs := vfs.NewVFSRoot(objectset)

	// Mount the filesystem
	mount, err := mount.NewMount(ctx, fs, *mountPoint)
	if err != nil {
		log.Fatal(err)
	}
	defer mount.Unmount()

	log.Printf("Mounted at %s", mount.MountPoint())

	fs.Print()

	// Handle signals for graceful unmounting.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down ipfs-go-storage...")
}
