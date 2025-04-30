// Package reposiroty implements a virtual filesystem that stores IPFS CIDs in a  tree-like structure.
package repository

import (
	"sync"
	"time"
)

// No actual file data is stored in the repository.
// Repository additionally maintains an index from CID to a path where it is stored

type Repository struct {
	mutex   sync.Mutex
	dirtree map[string][]DirEntry
}

type DirEntry interface {
	// String returns a description of the Object
	String() string

	// ModTime returns the modification date of the file
	ModTime() time.Time

	// Size returns the size of the file
	Size() int64
}

type Directory interface {
	DirEntry
}

type Object interface {
	DirEntry
}
