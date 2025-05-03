package ipfs

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"ipfs-go-storage/config"
	"ipfs-go-storage/ipfs/datastore"
	"time"

	"github.com/ipfs/boxo/blockservice"
	"github.com/ipfs/boxo/files"
	"github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/boxo/ipld/unixfs/importer/balanced"
	"github.com/ipfs/boxo/ipns"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multicodec"

	bsclient "github.com/ipfs/boxo/bitswap/client"
	bsnet "github.com/ipfs/boxo/bitswap/network/bsnet"
	blockstore "github.com/ipfs/boxo/blockstore"
	chunker "github.com/ipfs/boxo/chunker"
	unixfile "github.com/ipfs/boxo/ipld/unixfs/file"
	uih "github.com/ipfs/boxo/ipld/unixfs/importer/helpers"
	format "github.com/ipfs/go-ipld-format"
)

const IPFSFetchTimeout = 1 * time.Second
const IPFSHealthcheckTicker = 10 * time.Second

var (
	ErrorNotAFile     = errors.New("CID is not a UnixFile")
	ErrorFileTooLarge = errors.New("File is too large")
)

type bitswapInfo struct {
	privKey  crypto.PrivKey
	pubKey   crypto.PubKey
	peerInfo *peer.AddrInfo
}

type ipnsInfo struct {
	privKey    crypto.PrivKey
	pubKey     crypto.PubKey
	peerID     peer.ID
	ipnsKey    ipns.Name
	canPublish bool // Private Key valid
	canResolve bool // Public Key valid
}

type Connector struct {
	// Initial settings
	cfg *config.Config

	// Private and Public keys for a new LibP2P Host
	bitswap bitswapInfo

	// Private/Public keys + Peer ID for an IPNS Publisher
	// This info is shared amongst subscribers of the same VFS ObjectSet
	ipns ipnsInfo

	// Run-time state
	ticker *time.Ticker
	host   host.Host
	bswap  *bsclient.Client
	dsvc   format.DAGService
	evsub  event.Subscription
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
		cfg: cfg,
		bitswap: bitswapInfo{
			privKey:  priv,
			pubKey:   pub,
			peerInfo: info,
		},
		ipns: ipnsInfo{
			privKey:    cfg.Publisher.PrivKey.PrivKey,
			pubKey:     cfg.Publisher.PubKey.PubKey,
			canPublish: cfg.Publisher.PrivKey.Valid(),
			canResolve: cfg.Publisher.PubKey.Valid(),
		},
		done: make(chan bool),
	}

	// Verify that Publisher peerID matches the Private Key (if valid)
	if cfg.Publisher.PubKey.Valid() {
		pid, err := peer.IDFromPublicKey(cfg.Publisher.PubKey.PubKey)
		if err != nil {
			return nil, err
		}

		ipfs.ipns.peerID = pid
		ipfs.ipns.ipnsKey = ipns.NameFromPeer(pid)
		log.Infof("IPNS Key: %s", ipfs.ipns.ipnsKey.String())
	}

	return ipfs, nil
}

func (c *Connector) connectionHealthcheck(ctx context.Context) {
	log.Debugf("ipfs.Connector.connectionHealthcheck()")

	// for _, conn := range c.host.Network().Conns() {
	// 	log.Printf("Connection to %s: %v", conn.RemotePeer().String(), conn.Stat().Stats)
	// }

	// Check if we are connected to the target peer
	if c.host.Network().Connectedness(c.bitswap.peerInfo.ID) == network.NotConnected {
		log.Warningf("Not connected to %s, reconnecting...", c.bitswap.peerInfo.ID.String())
		if err := c.host.Connect(ctx, *c.bitswap.peerInfo); err == nil {
			log.Infof("Reconnected to %s", c.bitswap.peerInfo.String())
		}
	}
}

func (c *Connector) processEvent(ctx context.Context, evt interface{}) {
	switch evt.(type) {
	case event.EvtPeerConnectednessChanged:
		e := evt.(event.EvtPeerConnectednessChanged)
		log.Debugf("EvtPeerConnectednessChanged: %s -> %s\n", e.Peer.String(), e.Connectedness.String())
	case event.EvtLocalReachabilityChanged:
		e := evt.(event.EvtLocalReachabilityChanged)
		log.Debugf("EvtLocalReachabilityChanged: %s\n", e.Reachability.String())
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
			c.processEvent(ctx, evt)
		}
	}
}

// Start starts the IPFSConnector. On shutdown user must call Close()
func (c *Connector) Start(ctx context.Context) error {
	var err error

	// Connection manager
	connmgr, err := connmgr.NewConnManager(
		100, // Lowwater
		400, // HighWater,
		connmgr.WithGracePeriod(time.Minute),
	)

	// Basic LibP2P options
	opts := []libp2p.Option{
		libp2p.Identity(c.bitswap.privKey),
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", c.cfg.IPFS.Port),
			fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", c.cfg.IPFS.Port),
		),
		libp2p.ConnectionManager(connmgr),
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
	log.Infof("I am %s", thisAddr)

	// Datastore
	//ds := dsync.MutexWrap(datastore.NewMapDatastore())
	ds := datastore.NewMemCacheDatastore(1 * time.Minute)
	bs := blockstore.NewBlockstore(ds)
	bs = blockstore.NewIdStore(bs)

	// Create a new bitswap client
	bsn := bsnet.NewFromIpfsHost(c.host)
	c.bswap = bsclient.New(ctx, bsn, nil, bs)
	bsn.Start(c.bswap)

	bsrv := blockservice.New(bs, c.bswap)
	c.dsvc = merkledag.NewDAGService(bsrv)

	// Subscribe to event bus
	if c.evsub, err = c.host.EventBus().Subscribe(event.WildcardSubscription); err != nil {
		c.bswap.Close()
		c.host.Close()
		return err
	}

	// Create a ticker
	c.ticker = time.NewTicker(IPFSHealthcheckTicker)

	// Start the eventListener goroutine
	go c.eventListener(ctx)

	// Connect to the target peer
	if err = c.host.Connect(ctx, *c.bitswap.peerInfo); err != nil {
		log.Warningf("Failed to connect to %s: %s", c.bitswap.peerInfo.String(), err.Error())
		// This is not a fault.
	}

	// Start the goroutine to execute a graceful shutdown
	go func() {
		<-ctx.Done()
		log.Infof("Context cancelled, shutting down IPFS connector...")
		c.Close()
	}()

	return nil
}

func (c *Connector) Close() error {
	log.Debugf("ipfs.Connector.Close()")
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
	log.Debugf("ipfs.Connector.GetUnixfile(%s)", id.String())

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

func (c *Connector) FetchUnixFile(ctx context.Context, id cid.Cid) ([]byte, error) {
	log.Debugf("ipfs.Connector.FetchUnixFile(%s)", id.String())

	uf, err := c.GetUnixfile(ctx, id)
	if err != nil {
		return nil, err
	}
	defer uf.Close()

	if size, err := uf.Size(); err != nil {
		log.Errorf("ipfs.Connector.FetchUnixFile(%s): %v", id.String(), err)
		return nil, err
	} else if size > IPFSUnixFileMaxSize {
		log.Errorf("ipfs.Connector.FetchUnixFile(%s): File is too large (%d bytes)", id.String(), size)
		return nil, ErrorFileTooLarge
	}

	var buf bytes.Buffer
	if f, ok := uf.(files.File); ok {
		if _, err := io.Copy(&buf, f); err != nil {
			log.Errorf("ipfs.Connector.FetchUnixFile(%s): %v", id.String(), err)
			return nil, err
		}
	}

	log.Debugf("ipfs.Connector.FetchUnixFile(%s): %d bytes", id.String(), buf.Len())

	return buf.Bytes(), nil
}

func (c *Connector) StoreUnixFile(ctx context.Context, data []byte) (cid.Cid, error) {
	log.Debugf("ipfs.Connector.StoreUnixFile(%d bytes)", len(data))

	// Create a new UnixFS file from the data
	reader := bytes.NewReader(data)

	// Create a UnixFS graph from our file, parameters described here but can be visualized at https://dag.ipfs.tech/
	dbp := uih.DagBuilderParams{
		Maxlinks:  uih.DefaultLinksPerBlock, // Default max of 174 links per block
		RawLeaves: true,                     // Leave the actual file bytes untouched instead of wrapping them in a dag-pb protobuf wrapper
		CidBuilder: cid.V1Builder{ // Use CIDv1 for all links
			Codec:    uint64(multicodec.DagPb),
			MhType:   uint64(multicodec.Sha2_256), // Use SHA2-256 as the hash function
			MhLength: -1,                          // Use the default hash length for the given hash function (in this case 256 bits)
		},
		FileModTime: time.Now(),
		Dagserv:     c.dsvc,
		NoCopy:      false,
	}

	ufsBuilder, err := dbp.New(chunker.NewSizeSplitter(reader, chunker.DefaultBlockSize)) // Split the file up into fixed sized 256KiB chunks
	if err != nil {
		return cid.Undef, err
	}

	nd, err := balanced.Layout(ufsBuilder) // Arrange the graph with a balanced layout
	if err != nil {
		return cid.Undef, err
	}

	log.Debugf("ipfs.Connector.StoreUnixFile(): %s", nd.Cid().String())
	return nd.Cid(), nil
}
