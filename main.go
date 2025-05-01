package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"ipfs-go-storage/mount"
	"ipfs-go-storage/vfs"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	bsclient "github.com/ipfs/boxo/bitswap/client"
	bsnet "github.com/ipfs/boxo/bitswap/network/bsnet"
	"github.com/ipfs/boxo/blockservice"
	blockstore "github.com/ipfs/boxo/blockstore"
	"github.com/ipfs/boxo/files"
	"github.com/ipfs/boxo/ipld/merkledag"
	unixfile "github.com/ipfs/boxo/ipld/unixfs/file"
)

const ipfsFetchTimeout = 10 * time.Second

var (
	ipfsPeer   = flag.String("bitswap-peer", "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWPzN6y3VHiWVTSqf4R3yuEWjtaZhjDPZhiYH7q1bBiGVi", "IPFS peer address to connect to")
	p2pPort    = flag.Int("port", 4002, "Port for libP2P host")
	mountPoint = flag.String("mountpoint", "", "Path to mount the filesystem at")
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

type IPFSConnector struct {
}

type IPFSObject struct {
	// Connector to the IPFS BitSwap service
	ipfs *IPFSConnector

	// Persistent metadata
	pubTime time.Time // Time the object was published on the VFS

	// Metadata
	mu            sync.Mutex
	metadataReady bool
	cid           cid.Cid
	size          uint64
}

func (o *IPFSObject) refreshMetadata() {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.metadataReady {
		return
	}

	o.size = 1024

	o.metadataReady = true
}

func (o *IPFSObject) Size() uint64 {
	o.refreshMetadata()
	return o.size
}

func (o *IPFSObject) ModTime() time.Time {
	return o.pubTime
}

// main is the entry point of the application.
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Parse command-line options
	flag.Parse()
	log.Println("Starting ipfs-go-storage...")

	objectset := map[string]vfs.VFSObject{
		"/Movies/Happy Death Day (2017)/Schastlivogo_dnya_smerti_2017_HDRip_r5_[scarabey.org].mkv": &IPFSObject{},
		"/Movies/Venom (2018)/Веном_2018_BDRip.mkv":                                                &IPFSObject{},
	}

	fs := vfs.NewVFSRoot(objectset)

	mount, err := mount.NewMount(ctx, fs, *mountPoint)
	if err != nil {
		log.Fatal(err)
	}

	fs.Print()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Unmounting...")
	if err := mount.Unmount(); err != nil {
		log.Printf("Failed to unmount: %v", err)
	}

	return

	// Create LibP2P Host
	h, haddr, err := makeLibP2PHost(*p2pPort)
	if err != nil {
		log.Fatal(err)
	}
	defer h.Close()

	log.Printf("I am %s", haddr)

	bsn := bsnet.NewFromIpfsHost(h)
	bswap := bsclient.New(ctx, bsn, nil, blockstore.NewBlockstore(datastore.NewNullDatastore()))
	bsn.Start(bswap)
	defer bswap.Close()

	// Turn the targetPeer into a multiaddr.
	maddr, err := multiaddr.NewMultiaddr(*ipfsPeer)
	if err != nil {
		log.Fatal(err)
	}

	// Extract the peer ID from the multiaddr.
	info, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		log.Fatal(err)
	}

	// Directly connect to the peer that we know has the content.
	// Generally this peer will come from whatever content routing system is provided, however go-bitswap will also ask peers it is connected to for content so this will work.
	if err := h.Connect(ctx, *info); err != nil {
		log.Fatal(err)
	}
	log.Printf("Connected to %s", info.ID.String())

	// c, err := cid.Parse("QmSnuWmxptJZdLJpKRarxBMS2Ju2oANVrgbr2xWbie9b2D") // Directory
	c, err := cid.Parse("Qmc8mmzycvXnzgwBHokZQd97iWAmtdFMqX4FZUAQ5AQdQi") // File
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("CID: %s", c.String())

	dserv := merkledag.NewReadOnlyDagService(merkledag.NewSession(ctx, merkledag.NewDAGService(blockservice.New(blockstore.NewBlockstore(datastore.NewNullDatastore()), bswap))))

	fetchCtx, _ := context.WithTimeout(ctx, ipfsFetchTimeout)
	nd, err := dserv.Get(fetchCtx, c)
	if err != nil {
		log.Fatal(err)
	}

	// Now we need to figure out whether this is a file or a directory.
	// We can do this by checking the node type.

	uf, err := unixfile.NewUnixfsFile(ctx, dserv, nd)
	if err != nil {
		log.Fatal(err)
	}

	fsize, err := uf.Size()
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("File size: %d bytes", fsize)

	if f, ok := uf.(files.File); ok {
		f.Close()
	} else {
		log.Fatalf("expected a file")
	}
}
