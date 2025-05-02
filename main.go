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

func runInit(ctx context.Context, configPath string) {
	log.Println("Initializing ipfs-go-storage...")
}

func runMount(ctx context.Context, configPath string, mountPoint string, p2pPort int) {
	ctx, cancel := context.WithCancel(context.Background())

	log.Println("Mounting ipfs-go-storage...")

	ipfsPeer := "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWPzN6y3VHiWVTSqf4R3yuEWjtaZhjDPZhiYH7q1bBiGVi"

	// Create IPFS connector
	connector, err := ipfs.NewConnector(ipfsPeer, p2pPort)
	if err != nil {
		log.Fatal(err)
	}

	// Start IPFS
	if err := connector.Start(ctx); err != nil {
		log.Fatal(err)
	}

	// Build the VFS ObjectSet
	objectset, err := BuildObjectSet(ctx, connector)
	if err != nil {
		log.Fatal(err)
	}

	// Create the VFS
	fs := vfs.NewVFSRoot(objectset)

	// Mount the filesystem
	mount, err := mount.NewMount(ctx, fs, mountPoint)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Mounted at %s", mount.MountPoint())

	fs.Print()

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

func runPublish(ctx context.Context, configPath string) {
	log.Println("Publishing ipfs-go-storage...")
}

// main is the entry point of the application.
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	initCmd := flag.NewFlagSet("init", flag.ExitOnError)
	initConfig := initCmd.String("config", "", "Path to config file")

	mountCmd := flag.NewFlagSet("mount", flag.ExitOnError)
	mountConfig := mountCmd.String("config", "", "Path to config file")
	mountPoint := mountCmd.String("mountpoint", "/tmp/ipfs-go-storage", "Path to mount the filesystem at")
	mountP2PPort := mountCmd.Int("port", 4002, "Port for libP2P host")

	publishCmd := flag.NewFlagSet("publish", flag.ExitOnError)
	publishConfig := publishCmd.String("config", "", "Path to config file")

	if len(os.Args) < 2 {
		log.Fatal("Expected a subcommand")
	}

	switch os.Args[1] {
	case "init":
		initCmd.Parse(os.Args[2:])
		runInit(ctx, *initConfig)
	case "mount":
		mountCmd.Parse(os.Args[2:])
		runMount(ctx, *mountConfig, *mountPoint, *mountP2PPort)
	case "publish":
		publishCmd.Parse(os.Args[2:])
		runPublish(ctx, *publishConfig)
	default:
		log.Fatalf("Invalid subcommand '%s'", os.Args[1])
	}
}
