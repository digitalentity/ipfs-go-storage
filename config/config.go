package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log"
	"os"

	"github.com/libp2p/go-libp2p/core/crypto"
)

// Config represents the configuration for the ipfs-go-storage application
type Config struct {
	// Default config file location
	configFile string

	// Publisher settings define the IPNS address which will be used to manage the VFS
	Publisher struct {
		PrivKey *PrivKey `json:"priv_key"`
		PubKey  *PubKey  `json:"pub_key"`
	} `json:"publisher"`

	IPFS struct {
		PeerAddr string `json:"peer_addr"`
		Port     int    `json:"port"`
	} `json:"ipfs"`
}

// NewConfig generates a new configuration with default settings
func NewEmptyConfig(configFile string) *Config {
	cfg := &Config{}

	cfg.configFile = configFile

	cfg.Publisher.PrivKey = &PrivKey{}
	cfg.Publisher.PubKey = &PubKey{}

	cfg.IPFS.PeerAddr = "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWPzN6y3VHiWVTSqf4R3yuEWjtaZhjDPZhiYH7q1bBiGVi"
	cfg.IPFS.Port = 4002

	return cfg
}

func NewConfigFromFile(configFile string) (*Config, error) {
	cfg := NewEmptyConfig(configFile)
	if err := cfg.Load(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) GenerateKeys() error {
	// Generate a new key pair for the Publisher
	priv, pub, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, rand.Reader)
	if err != nil {
		return err
	}

	c.Publisher.PrivKey = &PrivKey{PrivKey: priv}
	c.Publisher.PubKey = &PubKey{PubKey: pub}
	return nil
}

// Save saves the configuration to a file

func (c *Config) Save() error {
	log.Printf("Saving config to %s", c.configFile)

	// We'll marshall our structure to JSON and write it into a file
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.configFile, data, 0644)
}

func (c *Config) Load() error {
	log.Printf("Loading config from %s", c.configFile)

	data, err := os.ReadFile(c.configFile)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, c); err != nil {
		return err
	}

	// Do some config validation and print the values
	if c.Publisher.PrivKey.Valid() {
		sk, _ := c.Publisher.PrivKey.PrivKey.Raw()
		log.Printf("Publisher.PrivKey: %s", base64.StdEncoding.EncodeToString(sk))
	}

	if c.Publisher.PubKey.Valid() {
		pk, _ := c.Publisher.PubKey.PubKey.Raw()
		log.Printf("Publisher.PubKey: %s", base64.StdEncoding.EncodeToString(pk))
	}

	return nil
}
