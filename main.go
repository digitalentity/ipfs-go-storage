package main

import (
	"context"
	"flag"
	"ipfs-go-storage/commands"
	"ipfs-go-storage/config"
	"os"

	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("main")

func RunPublish(ctx context.Context, cfg *config.Config) {
	log.Infof("Publishing ipfs-go-storage...")
}

func registerGlobalFlags(fset *flag.FlagSet) {
	flag.VisitAll(func(f *flag.Flag) {
		fset.Var(f.Value, f.Name, f.Usage)
	})
}

// main is the entry point of the application.
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	configFile := flag.String("config", "/tmp/ipfs-go-storage.json", "Path to config file")
	logLevel := flag.String("loglevel", "info", "Log level")

	initCmd := flag.NewFlagSet("init", flag.ExitOnError)
	registerGlobalFlags(initCmd)

	mountCmd := flag.NewFlagSet("mount", flag.ExitOnError)
	mountPoint := mountCmd.String("mountpoint", "/tmp/ipfs-go-storage", "Path to mount the filesystem at")
	registerGlobalFlags(mountCmd)

	publishCmd := flag.NewFlagSet("publish", flag.ExitOnError)
	registerGlobalFlags(publishCmd)

	if len(os.Args) < 2 {
		log.Fatal("Expected a subcommand")
	}
	cmd, args := os.Args[1], os.Args[2:]

	switch cmd {
	case "init":
		initCmd.Parse(args)
		logging.SetLogLevel("*", *logLevel)
		cfg := config.NewEmptyConfig(*configFile)
		commands.RunInit(ctx, cfg)
	case "mount":
		mountCmd.Parse(args)
		logging.SetLogLevel("*", *logLevel)
		cfg, err := config.NewConfigFromFile(*configFile)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
		commands.RunMount(ctx, cfg, *mountPoint)
	case "publish":
		publishCmd.Parse(args)
		logging.SetLogLevel("*", *logLevel)
		cfg, err := config.NewConfigFromFile(*configFile)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
		RunPublish(ctx, cfg)
	default:
		log.Fatalf("Invalid subcommand '%s'", os.Args[1])
	}
}
