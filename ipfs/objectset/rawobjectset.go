package objectset

import (
	"bytes"
	"encoding/gob"
	"ipfs-go-storage/ipfs"
	"ipfs-go-storage/vfs"
	"os"
	"time"
)

type RawObject struct {
	Cid   string
	Mtime time.Time
}

type RawObjectSet struct {
	Namespace  string
	LastUpdate time.Time
	Objects    map[string]RawObject
}

func NewRawObjectSet(namespace string) *RawObjectSet {
	return &RawObjectSet{
		Namespace:  namespace,
		LastUpdate: time.Now(),
		Objects:    make(map[string]RawObject),
	}
}

func NewRawObjectSetFromBytes(data []byte) (*RawObjectSet, error) {
	var raw RawObjectSet
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&raw); err != nil {
		return nil, err
	}
	return &raw, nil
}

func NewRawObjectSetFromFile(path string) (*RawObjectSet, error) {
	log.Debugf("ObjectSetWatcher.ReadFromFile()")
	data, err := os.ReadFile(path)
	if err != nil {
		log.Errorf("Failed to read ObjectSet from [%s]: %v", path, err)
		return nil, err
	}

	if len(data) == 0 {
		log.Errorf("Empty ObjectSet from [%s]", path)
		return nil, ErrorNoData
	}

	return NewRawObjectSetFromBytes(data)
}

func (ro *RawObjectSet) Marshall() ([]byte, error) {
	log.Debugf("ObjectSetWatcher.Marshall()")
	var b bytes.Buffer
	if err := gob.NewEncoder(&b).Encode(ro); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (ro *RawObjectSet) WriteToFile(path string) error {
	log.Debugf("ObjectSetWatcher.WriteToFile()")
	data, err := ro.Marshall()
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (ro *RawObjectSet) ObjectSet(connector *ipfs.Connector) (*ObjectSet, error) {
	objectset := make(map[string]vfs.VFSObject)

	for path, r := range ro.Objects {
		obj, err := ipfs.NewObject(connector, r.Cid, r.Mtime)
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
