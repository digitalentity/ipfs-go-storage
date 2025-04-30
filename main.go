package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"log"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/multiformats/go-multiaddr"
)

var (
	ipfsPeer = flag.String("bitswap-peer", "/ip4/127.0.0.1/tcp/4001/QmUtp8xEVgWC5dNPthF2g37eVvCdrqY1FPxLxXZoKkPbdp", "IPFS peer address to connect to")
	p2pPort  = flag.Int("port", 4002, "Port for libP2P host")
)

// makeLibP2PHost creates a new libp2p host with the given port.
func makeLibP2PHost(port int) (host.Host, string, error) {
	r := rand.Reader

	// Generate a new key pair for the host
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
	if err != nil {
		return nil, "", err
	}

	// Basic LibP2P options
	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port)),
		libp2p.Identity(priv),
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, "", err
	}

	hostAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/p2p/%s", h.ID().String()))
	if err != nil {
		return nil, "", err
	}

	addr := h.Addrs()[0]

	return h, addr.Encapsulate(hostAddr).String(), nil
}

// main is the entry point of the application.
func main() {
	_, cancel := context.WithCancel(context.Background())

	// Parse command-line options
	flag.Parse()

	log.Println("Starting ipfs-go-storage...")

	// Create LibP2P Host
	h, haddr, err := makeLibP2PHost(*p2pPort)
	if err != nil {
		log.Fatal(err)
	}
	defer h.Close()

	log.Printf("I am %s", haddr)

	defer cancel()
}
