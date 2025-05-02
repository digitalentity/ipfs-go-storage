package commands

import (
	"context"
	"ipfs-go-storage/config"
)

func RunInit(ctx context.Context, cfg *config.Config) {
	log.Infof("Initializing ipfs-go-storage...")

	if err := cfg.GenerateKeys(); err != nil {
		log.Fatalf("Failed to generate keys: %v", err)
	}

	if err := cfg.Save(); err != nil {
		log.Fatalf("Failed to save config: %v", err)
	}
}
