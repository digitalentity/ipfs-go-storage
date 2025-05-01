package ipfs

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"time"

	bsclient "github.com/ipfs/boxo/bitswap/client"
	bsnet "github.com/ipfs/boxo/bitswap/network/bsnet"
	"github.com/ipfs/boxo/blockservice"
	blockstore "github.com/ipfs/boxo/blockstore"
	"github.com/ipfs/boxo/files"
	"github.com/ipfs/boxo/ipld/merkledag"
	unixfile "github.com/ipfs/boxo/ipld/unixfs/file"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	format "github.com/ipfs/go-ipld-format"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

const IPFSFetchTimeout = 1 * time.Second

var (
	ErrorNotAFile = errors.New("CID is not a UnixFile")
)

type IPFSConnector struct {
	// Initial settings
	port int

	// Private and Public keys for a new LibP2P Host
	privKey  crypto.PrivKey
	pubKey   crypto.PubKey
	peerInfo *peer.AddrInfo

	// Run-time state
	host  host.Host
	bswap *bsclient.Client
	dsvc  format.DAGService
}

// NewIPFSConnector creates a new IPFSConnector.
func NewIPFSConnector(peerAddr string, listenPort int) (*IPFSConnector, error) {
	r := rand.Reader

	// Turn the targetPeer into a multiaddr.
	maddr, err := multiaddr.NewMultiaddr(peerAddr)
	if err != nil {
		return nil, err
	}

	// Extract the peer ID from the multiaddr.
	info, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		return nil, err
	}

	// Generate a new key pair for the host
	priv, pub, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
	if err != nil {
		return nil, err
	}

	ipfs := &IPFSConnector{
		port:     listenPort,
		peerInfo: info,
		privKey:  priv,
		pubKey:   pub,
	}

	return ipfs, nil
}

// Start starts the IPFSConnector. On shutdown user must call Close()
func (c *IPFSConnector) Start(ctx context.Context) error {
	var err error

	// Basic LibP2P options
	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", c.port)),
		libp2p.Identity(c.privKey),
	}

	// Start a new libp2p Host
	c.host, err = libp2p.New(opts...)
	if err != nil {
		return err
	}

	hostAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/p2p/%s", c.host.ID().String()))
	if err != nil {
		c.host.Close()
		return err
	}

	thisAddr := c.host.Addrs()[0].Encapsulate(hostAddr).String()
	log.Printf("I am %s", thisAddr)

	// Create a new bitswap client
	bsn := bsnet.NewFromIpfsHost(c.host)
	c.bswap = bsclient.New(ctx, bsn, nil, blockstore.NewBlockstore(datastore.NewNullDatastore()))
	bsn.Start(c.bswap)

	// Connect to the target peer
	if err = c.host.Connect(ctx, *c.peerInfo); err != nil {
		c.bswap.Close()
		c.host.Close()
		return err
	}

	log.Printf("Connected to %s", c.peerInfo.String())

	// Create a new DAGService
	c.dsvc = merkledag.NewReadOnlyDagService(merkledag.NewSession(ctx, merkledag.NewDAGService(blockservice.New(blockstore.NewBlockstore(datastore.NewNullDatastore()), c.bswap))))

	return nil
}

func (c *IPFSConnector) Close() error {
	c.bswap.Close()
	c.host.Close()
	return nil
}

func (c *IPFSConnector) GetUnixfile(ctx context.Context, id cid.Cid) (files.Node, error) {
	log.Printf("IPFSConnector.GetUnixfile(%s)", id.String())

	fetchCtx, _ := context.WithTimeout(ctx, IPFSFetchTimeout)
	node, err := c.dsvc.Get(fetchCtx, id)
	if err != nil {
		return nil, err
	}

	uf, err := unixfile.NewUnixfsFile(ctx, c.dsvc, node)
	if err != nil {
		return nil, err
	}

	if _, ok := uf.(files.File); !ok {
		return nil, ErrorNotAFile
	}

	return uf, nil
}
