package ipfs

import "time"

const (
	ObjectSetWatcherInterval = 60 * time.Second

	IPFSObjectSetTimeout   = 10 * time.Second // Wait 10 seconds for ObjectSet retreival
	IPFSObjectErrorTimeout = 10 * time.Second
	IPFSUnixFileTimeout    = 60 * time.Second // Cache open file object for 60 seconds
	IPFSUnixFileMaxSize    = 1 * 1024 * 1024  // 1MB for direct fetch
)
