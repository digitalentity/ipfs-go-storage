package datastore

import (
	"context"
	"sync"
	"time"

	ipfsds "github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("datastore")

type item struct {
	value      []byte
	expiration time.Time
}

type MemCacheDatastore struct {
	mu     sync.Mutex
	ttl    time.Duration
	values map[ipfsds.Key]item
}

var _ ipfsds.Datastore = (*MemCacheDatastore)(nil)
var _ ipfsds.Batching = (*MemCacheDatastore)(nil)

func NewMemCacheDatastore(ttl time.Duration) *MemCacheDatastore {
	m := &MemCacheDatastore{
		mu:     sync.Mutex{},
		ttl:    ttl,
		values: make(map[ipfsds.Key]item, 1024),
	}

	go m.collectGarbage()
	return m
}

func (m *MemCacheDatastore) collectGarbage() {
	for {
		time.Sleep(60 * time.Second)
		m.mu.Lock()
		log.Debugf("Collecting expired keys. Total keys: %d", len(m.values))
		now := time.Now()
		for k, v := range m.values {
			if v.expiration.Before(now) {
				log.Debugf("Evicting expired key %s", k)
				delete(m.values, k)
			}
		}
		m.mu.Unlock()
	}
}

func (m *MemCacheDatastore) Put(ctx context.Context, key ipfsds.Key, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[key] = item{value: value, expiration: time.Now().Add(m.ttl)}
	return nil
}

func (m *MemCacheDatastore) Get(ctx context.Context, key ipfsds.Key) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.values[key]; ok {
		v.expiration = time.Now().Add(m.ttl)
		return v.value, nil
	}
	return nil, ipfsds.ErrNotFound
}

func (m *MemCacheDatastore) Has(ctx context.Context, key ipfsds.Key) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.values[key]; ok {
		return true, nil
	}
	return false, nil
}

func (m *MemCacheDatastore) Delete(ctx context.Context, key ipfsds.Key) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.values, key)
	return nil
}

func (m *MemCacheDatastore) Query(ctx context.Context, q dsq.Query) (dsq.Results, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	re := make([]dsq.Entry, 0, len(m.values))
	for k, v := range m.values {
		e := dsq.Entry{Key: k.String(), Size: len(v.value)}
		if !q.KeysOnly {
			e.Value = v.value
		}
		re = append(re, e)
	}
	r := dsq.ResultsWithEntries(q, re)
	r = dsq.NaiveQueryApply(q, r)
	return r, nil
}

func (m *MemCacheDatastore) GetSize(ctx context.Context, key ipfsds.Key) (size int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.values[key]; ok {
		return len(v.value), nil
	}
	return -1, ipfsds.ErrNotFound
}

func (m *MemCacheDatastore) Close() error {
	return nil
}

func (m *MemCacheDatastore) Sync(ctx context.Context, prefix ipfsds.Key) error {
	return nil
}

func (m *MemCacheDatastore) Batch(ctx context.Context) (ipfsds.Batch, error) {
	return ipfsds.NewBasicBatch(m), nil
}
