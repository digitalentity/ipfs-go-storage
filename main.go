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

type IPFSFileHandle struct {
	obj  *IPFSObject
	mu   sync.Mutex
	file files.File
}

func (fh *IPFSFileHandle) String() string {
	return fh.obj.String()
}

func (fh *IPFSFileHandle) Close(ctx context.Context) error {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	log.Printf("IPFSFileHandle.Close(%s)", fh)
	return fh.file.Close()
}

func (fh *IPFSFileHandle) Read(ctx context.Context, dest []byte, off int64) (int, error) {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	log.Printf("IPFSFileHandle.Read(%s)", fh)

	_, err := fh.file.Seek(off, io.SeekStart)
	if err != nil {
		return 0, fmt.Errorf("Read error [%w]", err)
	}
	n, err := fh.file.Read(dest)
	if err == io.EOF {
		err = nil
	}
	return n, err
}

func (fh *IPFSFileHandle) Size(ctx context.Context) (int64, error) {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	log.Printf("IPFSFileHandle.Size(%s)", fh)
	return fh.file.Size()
}

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

func (o *IPFSObject) open(ctx context.Context) (*IPFSFileHandle, error) {
	log.Printf("IPFSObject.open(%s)", o)

	// Return the last error if it did not expire yet.
	if o.lastError != nil && o.errorTimeout.After(time.Now()) {
		return nil, o.lastError
	}

	uf, err := o.ipfs.GetUnixfile(ctx, o.cid)
	if err != nil {
		log.Printf("IPFSObject.open(%s): %v", o, err)
		o.lastError = fmt.Errorf("Open error [%w]", err)
		o.errorTimeout = time.Now().Add(IPFSObjectErrorTimeout)
		return nil, o.lastError
	}

	fh := &IPFSFileHandle{
		obj:  o,
		file: uf.(files.File),
	}

	return fh, nil
}

// func (o *IPFSObject) read(ctx context.Context, dest []byte, off int64) (int, error) {
// 	if o.file == nil {
// 		return 0, fmt.Errorf("Read error: file is not open")
// 	}

// 	_, err := o.file.Seek(off, io.SeekStart)
// 	if err != nil {
// 		return 0, fmt.Errorf("Read error [%w]", err)
// 	}

// 	n, err := o.file.Read(dest)
// 	if err == io.EOF {
// 		err = nil
// 	}

// 	if err != nil {
// 		return n, fmt.Errorf("Read error [%w]", err)
// 	}

// 	return n, nil
// }

func (o *IPFSObject) fetchAttributes(ctx context.Context) error {
	if o.attrValid {
		return nil
	}

	fh, err := o.open(ctx)
	if err != nil {
		return err
	}

	size, err := fh.Size(ctx)
	if err != nil {
		return err
	}

	o.attr.Size = uint64(size)
	o.attrValid = true

	fh.Close(ctx)
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

func (o *IPFSObject) Open(ctx context.Context) (vfs.VFSObjectHandle, error) {
	log.Printf("IPFSObject.Open(%s)", o)
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.open(ctx)
}

// func (o *IPFSObject) Read(ctx context.Context, dest []byte, off int64) (int, error) {
// 	o.mu.Lock()
// 	defer o.mu.Unlock()
// 	return o.read(ctx, dest, off)
// }

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
