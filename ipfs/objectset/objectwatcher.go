package objectset

import (
	"context"
	"errors"
	"ipfs-go-storage/config"
	"ipfs-go-storage/ipfs"
	"sync"
	"time"
)

var ErrorObjctSetNotNewer = errors.New("ObjectSet is not newer than current")

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
			Objects:    make(map[string]RawObject),
		},
		mu:   sync.Mutex{},
		c:    make(chan *ObjectSet),
		done: make(chan bool),
	}

	return osw
}

func (o *ObjectSetWatcher) processUpdate(ctx context.Context, newros *RawObjectSet) error {
	log.Debugf("ObjectSetWatcher.processUpdate()")

	o.mu.Lock()
	defer o.mu.Unlock()

	// Check that the RawObjectSet is newer
	if !newros.LastUpdate.After(o.ros.LastUpdate) {
		return ErrorObjctSetNotNewer
	}

	log.Infof("Got an updated ObjectSet: %v", newros.LastUpdate)

	// Transform to an in-memory ObjectSet
	nos, err := newros.ObjectSet(o.connector)
	if err != nil {
		log.Errorf("Failed to create new ObjectSet: %v", err)
		return err
	}

	// Marshall and save to file
	if err := newros.WriteToFile(o.cfg.ObjectSet.Path); err != nil {
		log.Errorf("Failed to save ObjectSet: %v", err)
		return err
	}

	// Send to the subscriber
	o.ros = newros
	o.c <- nos
	return nil
}

func (o *ObjectSetWatcher) fetchAndProcessUpdate(ctx context.Context) error {
	log.Debugf("ObjectSetWatcher.fetchAndProcessUpdate()")

	ros, err := NewRawObjectSetFromIPNS(ctx, o.connector, o.connector.GetIPNSKey())
	if err != nil {
		return err
	}

	if err := o.processUpdate(ctx, ros); err != nil {
		return err
	}

	return nil
}

func (o *ObjectSetWatcher) Start(ctx context.Context) error {
	// testros := &rawObjectSet{
	// 	Namespace:  "default",
	// 	LastUpdate: time.Now(),
	// 	Objects: map[string]rawObject{
	// 		"/Anime/Kusuriya no Hitorigoto TV-2 01.mkv": {Cid: "QmRR2wi98aHLfGf8Nu5MxM33BTrChyaQ9phNCHH2RF78WC", Mtime: time.Now()},
	// 	},
	// }
	// o.processUpdate(ctx, testros)

	// Start the goroutine to execute a graceful shutdown
	go func(ctx context.Context) {
		// Read initial RawObjectSet from file
		if ros, err := NewRawObjectSetFromFile(o.cfg.ObjectSet.Path); err == nil {
			o.processUpdate(ctx, ros) // We don't care about the error at this point
		}

		// Start first update immediately
		o.fetchAndProcessUpdate(ctx)
		o.Publish(ctx)

		// Now do the polling loop for updates
		o.ticker = time.NewTicker(ipfs.ObjectSetWatcherInterval)

		for {
			select {
			case <-o.ticker.C:
				o.fetchAndProcessUpdate(ctx)
				o.Publish(ctx)
			case <-ctx.Done():
				log.Infof("Context cancelled, shutting down IPFS connector...")
				o.Stop()
			}
		}
	}(ctx)

	return nil
}

func (o *ObjectSetWatcher) Publish(ctx context.Context) {
	log.Infof("ObjectSetWatcher.publish()")
	// Publish the existing objectset on the IPFS
	bt, err := o.ros.Marshall()
	if err != nil {
		log.Errorf("Failed to marshall ObjectSet: %v", err)
		return
	}

	cid, err := o.connector.StoreUnixFile(ctx, bt)
	if err != nil {
		log.Errorf("Failed to store ObjectSet: %v", err)
		return
	}

	log.Infof("Stored new ObjectSet on IPFS: %s", cid.String())
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
