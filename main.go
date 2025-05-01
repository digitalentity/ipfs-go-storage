package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"ipfs-go-storage/ipfs"
	"ipfs-go-storage/mount"
	"ipfs-go-storage/vfs"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ipfs/boxo/files"
	"github.com/ipfs/go-cid"
)

const IPFSObjectErrorTimeout = 10 * time.Second
const IPFSUnixFileTimeout = 60 * time.Second // Cache open file object for 60 seconds

var (
	ipfsPeer   = flag.String("bitswap-peer", "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWPzN6y3VHiWVTSqf4R3yuEWjtaZhjDPZhiYH7q1bBiGVi", "IPFS peer address to connect to")
	p2pPort    = flag.Int("port", 4002, "Port for libP2P host")
	mountPoint = flag.String("mountpoint", "", "Path to mount the filesystem at")
)

type IPFSObject struct {
	// Connector to the IPFS BitSwap service
	ipfs *ipfs.IPFSConnector

	// Object CID and VFS metadata
	cid cid.Cid

	// Run-time object state
	mu        sync.Mutex
	attrValid bool              // Attributes fetch was successful. VFS is read-only, so this needs to happen only once.
	attr      vfs.VFSObjectAttr // Cached attributes

	lastError    error     // Cached error
	errorTimeout time.Time // Error timeout

	fileTimeout time.Time  // Opened file timeout
	file        files.File // A handle to the file (when opened)
}

func NewIPFSObject(ipfs *ipfs.IPFSConnector, id string) (*IPFSObject, error) {
	c, err := cid.Parse(id)
	if err != nil {
		return nil, err
	}

	return &IPFSObject{
		ipfs:      ipfs,
		cid:       c,
		attr:      vfs.VFSObjectAttr{ModTime: time.Now()},
		attrValid: false,
	}, nil
}

func (o *IPFSObject) open(ctx context.Context) error {
	log.Printf("IPFSObject.open(%s)", o)

	// Check if the file is already open.
	if o.file != nil {
		return nil
	}

	// Sanity check: this should never happen
	if o.fileOpenCount > 0 {
		log.Panicf("IPFSObject.open: fileOpenCount > 0 but file pointer is nil")
	}

	// Return the last error if it did not expire yet.
	if o.lastError != nil && o.errorTimeout.After(time.Now()) {
		return o.lastError
	}

	uf, err := o.ipfs.GetUnixfile(ctx, o.cid)
	if err != nil {
		log.Printf("IPFSObject.open(%s): %v", o, err)
		o.lastError = fmt.Errorf("Open error [%w]", err)
		o.errorTimeout = time.Now().Add(IPFSObjectErrorTimeout)
		return o.lastError
	}

	o.file = uf.(files.File)
	o.fileOpenCount = 1

	log.Printf("IPFSObject.open(%s) success", o)
	return nil
}

func (o *IPFSObject) close(ctx context.Context) error {
	if o.file == nil {
		return nil
	}

	// Sanity check
	if o.fileOpenCount <= 0 && o.file != nil {
		log.Panicf("IPFSObject.close: fileOpenCount <= 0 but file pointer is not nil")
	}

	o.fileOpenCount--
	if o.fileOpenCount > 0 {
		return nil
	}

	// Close the file. Invalidate the file object even if we had an error here
	err := o.file.Close()
	o.file = nil

	if err != nil {
		log.Printf("IPFSObject.close(%s): %v", o, err)
		return err
	}

	log.Printf("IPFSObject.close(%s) success", o)
	return nil
}

func (o *IPFSObject) read(ctx context.Context, dest []byte, off int64) (int, error) {
	if o.file == nil {
		return 0, fmt.Errorf("Read error: file is not open")
	}

	_, err := o.file.Seek(off, io.SeekStart)
	if err != nil {
		return 0, fmt.Errorf("Read error [%w]", err)
	}

	n, err := o.file.Read(dest)
	if err == io.EOF {
		err = nil
	}

	if err != nil {
		return n, fmt.Errorf("Read error [%w]", err)
	}

	return n, nil
}

func (o *IPFSObject) fetchAttributes(ctx context.Context) error {
	if o.attrValid {
		return nil
	}

	if err := o.open(ctx); err != nil {
		return err
	}

	size, err := o.file.Size()
	if err != nil {
		return err
	}

	o.attr.Size = uint64(size)
	o.attrValid = true

	if err := o.close(ctx); err != nil {
		return err
	}

	return nil
}

func (o *IPFSObject) String() string {
	return o.cid.String()
}

func (o *IPFSObject) GetAttr(ctx context.Context) (*vfs.VFSObjectAttr, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.fetchAttributes(ctx); err != nil {
		return nil, err
	}
	return &o.attr, nil
}

func (o *IPFSObject) Open(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.open(ctx)
}

func (o *IPFSObject) Close(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.close(ctx)
}

func (o *IPFSObject) Read(ctx context.Context, dest []byte, off int64) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.read(ctx, dest, off)
}

func BuildObjectSet(ctx context.Context, ipfs *ipfs.IPFSConnector) (map[string]vfs.VFSObject, error) {
	objectset := make(map[string]vfs.VFSObject)

	data := map[string]string{
		"/Anime/Kusuriya no Hitorigoto TV-2 01.mkv": "QmRR2wi98aHLfGf8Nu5MxM33BTrChyaQ9phNCHH2RF78WC",
	}

	for path, id := range data {
		obj, err := NewIPFSObject(ipfs, id)
		if err != nil {
			return nil, err
		}

		objectset[path] = obj
	}

	return objectset, nil
}

// main is the entry point of the application.
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Parse command-line options
	flag.Parse()
	log.Println("Starting ipfs-go-storage...")

	// Create IPFS connector
	ipfs, err := ipfs.NewIPFSConnector(*ipfsPeer, *p2pPort)
	if err != nil {
		log.Fatal(err)
	}

	// Start IPFS
	if err := ipfs.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer ipfs.Close()

	// Build the VFS ObjectSet
	objectset, err := BuildObjectSet(ctx, ipfs)
	if err != nil {
		log.Fatal(err)
	}

	// Create the VFS
	fs := vfs.NewVFSRoot(objectset)

	// Mount the filesystem
	mount, err := mount.NewMount(ctx, fs, *mountPoint)
	if err != nil {
		log.Fatal(err)
	}
	defer mount.Unmount()

	log.Printf("Mounted at %s", mount.MountPoint())

	fs.Print()

	// Handle signals for graceful unmounting.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down ipfs-go-storage...")
}
