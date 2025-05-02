package main

import (
	"context"
	"flag"
	"ipfs-go-storage/commands"
	"ipfs-go-storage/config"
	"log"
	"os"
)

func RunPublish(ctx context.Context, cfg *config.Config) {
	log.Println("Publishing ipfs-go-storage...")
}

// main is the entry point of the application.
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	configFile := flag.String("config", "/tmp/ipfs-go-storage.yaml", "Path to config file")

	initCmd := flag.NewFlagSet("init", flag.ExitOnError)

	mountCmd := flag.NewFlagSet("mount", flag.ExitOnError)
	mountPoint := mountCmd.String("mountpoint", "/tmp/ipfs-go-storage", "Path to mount the filesystem at")
	mountP2PPort := mountCmd.Int("port", 4002, "Port for libP2P host")

	publishCmd := flag.NewFlagSet("publish", flag.ExitOnError)

	if len(os.Args) < 2 {
		log.Fatal("Expected a subcommand")
	}

	switch os.Args[1] {
	case "init":
		initCmd.Parse(os.Args[2:])
		cfg := config.NewEmptyConfig(*configFile)
		commands.RunInit(ctx, cfg)
	case "mount":
		mountCmd.Parse(os.Args[2:])
		cfg, err := config.NewConfigFromFile(*configFile)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
		commands.RunMount(ctx, cfg, *mountPoint, *mountP2PPort)
	case "publish":
		publishCmd.Parse(os.Args[2:])
		cfg, err := config.NewConfigFromFile(*configFile)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
		RunPublish(ctx, cfg)
	default:
		log.Fatalf("Invalid subcommand '%s'", os.Args[1])
	}
}
