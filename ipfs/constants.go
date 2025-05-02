package ipfs

import "time"

const IPFSObjectErrorTimeout = 10 * time.Second
const IPFSUnixFileTimeout = 60 * time.Second // Cache open file object for 60 seconds
