package objectset

import (
	"bytes"
	"context"
	"encoding/gob"
	"ipfs-go-storage/config"
	"ipfs-go-storage/ipfs"
	"ipfs-go-storage/vfs"
	"os"
	"sync"
	"time"

	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("ipfs/objectset")

type ObjectSet struct {
	LastUpdate time.Time
	Objects    map[string]vfs.VFSObject
}

type RawObjectSet struct {
	Namespace  string
	LastUpdate time.Time
	Objects    map[string]string
}

func UnmarshallRawObjectSet(data []byte) (*RawObjectSet, error) {
	var raw RawObjectSet
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&raw); err != nil {
		return nil, err
	}
	return &raw, nil
}

func MarshallRawObjectSet(raw *RawObjectSet) ([]byte, error) {
	var b bytes.Buffer
	if err := gob.NewEncoder(&b).Encode(raw); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (ro *RawObjectSet) ObjectSet(connector *ipfs.Connector) (*ObjectSet, error) {
	objectset := make(map[string]vfs.VFSObject)

	for path, id := range ro.Objects {
		obj, err := ipfs.NewObject(connector, id)
		if err != nil {
			return nil, err
		}
		objectset[path] = obj
	}

	return &ObjectSet{
		LastUpdate: ro.LastUpdate,
		Objects:    objectset,
	}, nil
}

type ObjectSetWatcher struct {
	// Configuration
	cfg *config.Config

	// IPFS Connector
	connector *ipfs.Connector

	// Cached RawObjectSet
	mu  sync.Mutex
	ros *RawObjectSet

	// We deliver received objectsets here
	c      chan *ObjectSet
	ticker *time.Ticker
	done   chan bool
}

func NewWatcher(cfg *config.Config, ipfs *ipfs.Connector) *ObjectSetWatcher {
	osw := &ObjectSetWatcher{
		cfg:       cfg,
		connector: ipfs,
		ros: &RawObjectSet{
			Namespace:  "default",
			LastUpdate: time.Unix(0, 0),
			Objects:    make(map[string]string),
		},
		mu:   sync.Mutex{},
		c:    make(chan *ObjectSet),
		done: make(chan bool),
	}

	return osw
}

func (o *ObjectSetWatcher) processUpdate(ctx context.Context, newros *RawObjectSet) {
	log.Debugf("ObjectSetWatcher.processUpdate()")

	o.mu.Lock()
	defer o.mu.Unlock()

	// Check that the RawObjectSet is newer
	if !newros.LastUpdate.After(o.ros.LastUpdate) {
		return
	}

	log.Infof("Got an updated ObjectSet: %v", newros.LastUpdate)

	// Transform to an in-memory ObjectSet
	nos, err := newros.ObjectSet(o.connector)
	if err != nil {
		log.Errorf("Failed to create new ObjectSet: %v", err)
		return
	}

	// Marshall and save to file
	data, err := MarshallRawObjectSet(newros)
	if err != nil {
		log.Errorf("Failed to marshall ObjectSet: %v", err)
		return
	}

	// Save to file
	if err := os.WriteFile(o.cfg.ObjectSet.Path, data, 0644); err != nil {
		log.Errorf("Failed to write ObjectSet to [%s]: %v", o.cfg.ObjectSet.Path, err)
		return
	}

	// Send to the subscriber
	o.ros = newros
	o.c <- nos
}

func (o *ObjectSetWatcher) update(ctx context.Context) {
	log.Infof("ObjectSetWatcher.update()")
	// TODO
}

func (o *ObjectSetWatcher) Start(ctx context.Context) error {
	// testros := &RawObjectSet{
	// 	Namespace:  "default",
	// 	LastUpdate: time.Now(),
	// 	Objects: map[string]string{
	// 		"/Anime/Kusuriya no Hitorigoto TV-2 01.mkv": "QmRR2wi98aHLfGf8Nu5MxM33BTrChyaQ9phNCHH2RF78WC",
	// 	},
	// }
	// o.processUpdate(ctx, testros)

	// Start the goroutine to execute a graceful shutdown
	go func(ctx context.Context) {
		// Read initial objectset from file cfg.ObjectSet.Path
		data, err := os.ReadFile(o.cfg.ObjectSet.Path)
		if err != nil {
			log.Errorf("Failed to read ObjectSet from [%s]: %v", o.cfg.ObjectSet.Path, err)
		} else {
			if ros, err := UnmarshallRawObjectSet(data); err != nil {
				log.Errorf("Failed to unmarshall ObjectSet: %v", err)
			} else {
				log.Infof("Loaded ObjectSet from [%s]", o.cfg.ObjectSet.Path)
				o.processUpdate(ctx, ros)
			}
		}

		// Start first update immediately
		o.update(ctx)

		// Now do the polling loop for updates
		o.ticker = time.NewTicker(ipfs.ObjectSetWatcherInterval)

		for {
			select {
			case <-o.ticker.C:
				o.update(ctx)
			case <-ctx.Done():
				log.Infof("Context cancelled, shutting down IPFS connector...")
				o.Stop()
			}
		}
	}(ctx)

	return nil
}

func (o *ObjectSetWatcher) Recv() chan *ObjectSet {
	return o.c
}

func (o *ObjectSetWatcher) Stop() error {
	o.ticker.Stop()
	o.done <- true
	return nil
}

func (o *ObjectSetWatcher) WaitDone() error {
	<-o.done
	return nil
}
