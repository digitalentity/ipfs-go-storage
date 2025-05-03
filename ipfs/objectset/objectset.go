package objectset

import (
	"errors"
	"ipfs-go-storage/vfs"
	"time"

	logging "github.com/ipfs/go-log/v2"
)

var ErrorNoData = errors.New("no data")

var log = logging.Logger("ipfs/objectset")

type ObjectSet struct {
	LastUpdate time.Time
	Objects    map[string]vfs.VFSObject
}
