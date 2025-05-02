package ipfs

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"ipfs-go-storage/config"
	"log"
	"time"

	"github.com/ipfs/boxo/blockservice"
	"github.com/ipfs/boxo/files"
	"github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	bsclient "github.com/ipfs/boxo/bitswap/client"
	bsnet "github.com/ipfs/boxo/bitswap/network/bsnet"
	blockstore "github.com/ipfs/boxo/blockstore"
	unixfile "github.com/ipfs/boxo/ipld/unixfs/file"
	format "github.com/ipfs/go-ipld-format"
	eventbus "github.com/libp2p/go-libp2p/core/event"
)

const IPFSFetchTimeout = 1 * time.Second
const IPFSHealthcheckTicker = 10 * time.Second

var (
	ErrorNotAFile = errors.New("CID is not a UnixFile")
)

type Connector struct {
	// Initial settings
	cfg *config.Config

	// Private and Public keys for a new LibP2P Host
	privKey  crypto.PrivKey
	pubKey   crypto.PubKey
	peerInfo *peer.AddrInfo

	// Run-time state
	ticker *time.Ticker
	host   host.Host
	bswap  *bsclient.Client
	dsvc   format.DAGService
	evsub  eventbus.Subscription
	done   chan bool
}

// NewConnector creates a new IPFSConnector.
func NewConnector(cfg *config.Config) (*Connector, error) {
	r := rand.Reader

	// Turn the targetPeer into a multiaddr.
	maddr, err := multiaddr.NewMultiaddr(cfg.IPFS.PeerAddr)
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

	ipfs := &Connector{
		cfg:      cfg,
		peerInfo: info,
		privKey:  priv,
		pubKey:   pub,
		done:     make(chan bool),
	}

	return ipfs, nil
}

func (c *Connector) connectionHealthcheck(ctx context.Context) {
	log.Printf("ipfs.Connector.connectionHealthcheck()")

	// for _, conn := range c.host.Network().Conns() {
	// 	log.Printf("Connection to %s: %v", conn.RemotePeer().String(), conn.Stat().Stats)
	// }

	// Check if we are connected to the target peer
	if c.host.Network().Connectedness(c.peerInfo.ID) == network.NotConnected {
		log.Printf("Not connected to %s, reconnecting...", c.peerInfo.ID.String())
		if err := c.host.Connect(ctx, *c.peerInfo); err == nil {
			log.Printf("Reconnected to %s", c.peerInfo.String())
		}
	}
}

func (c *Connector) eventListener(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.ticker.C:
			c.connectionHealthcheck(ctx)
		case evt := <-c.evsub.Out():
			log.Printf("Event: %v", evt)
		}
	}
}

// Start starts the IPFSConnector. On shutdown user must call Close()
func (c *Connector) Start(ctx context.Context) error {
	var err error

	// Basic LibP2P options
	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", c.cfg.IPFS.Port)),
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

	// Create a new DAGService
	c.dsvc = merkledag.NewReadOnlyDagService(merkledag.NewSession(ctx, merkledag.NewDAGService(blockservice.New(blockstore.NewBlockstore(datastore.NewNullDatastore()), c.bswap))))

	// Subscribe to event bus
	if c.evsub, err = c.host.EventBus().Subscribe(eventbus.WildcardSubscription); err != nil {
		c.bswap.Close()
		c.host.Close()
		return err
	}

	// Create a ticker
	c.ticker = time.NewTicker(IPFSHealthcheckTicker)

	// Start the eventListener goroutine
	go c.eventListener(ctx)

	// Connect to the target peer
	if err = c.host.Connect(ctx, *c.peerInfo); err != nil {
		log.Printf("Failed to connect to %s: %s", c.peerInfo.String(), err.Error())
		// c.bswap.Close()
		// c.host.Close()
		// return err
	} else {
		log.Printf("Connected to %s", c.peerInfo.String())
	}

	// Start the goroutine to execute a graceful shutdown
	go func() {
		<-ctx.Done()
		log.Printf("Context cancelled, shutting down IPFS connector...")
		c.Close()
	}()

	return nil
}

func (c *Connector) Close() error {
	log.Printf("ipfs.Connector.Close()")
	c.evsub.Close()
	c.bswap.Close()
	c.host.Close()
	c.ticker.Stop()
	c.done <- true
	return nil
}

func (c *Connector) WaitDone() error {
	select {
	case <-c.done:
		return nil
	case <-time.After(time.Second * 5):
		return fmt.Errorf("timeout waiting for IPFS connector to close")
	}
}

func (c *Connector) GetUnixfile(ctx context.Context, id cid.Cid) (files.Node, error) {
	log.Printf("ipfs.Connector.GetUnixfile(%s)", id.String())

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
