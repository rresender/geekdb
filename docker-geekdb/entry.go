package main

import "sync"

type id int

const (
	nameKey id = iota
)

// Entry default object
type Entry struct {
	value    int
	version  int
	numMutex sync.RWMutex
}

// InitEntry : Initialize an entry
func InitEntry(val int) *Entry {
	return &Entry{
		value: val,
	}
}

func (n *Entry) setValue(newVal int) {
	n.numMutex.Lock()
	defer n.numMutex.Unlock()
	n.value = newVal
	n.version = n.version + 1
}

func (n *Entry) getValue() (int, int) {
	n.numMutex.RLock()
	defer n.numMutex.RUnlock()
	return n.value, n.version
}

func (n *Entry) notifyValue(curVal int, curVersion int) bool {
	if curVersion > n.version {
		n.numMutex.Lock()
		defer n.numMutex.Unlock()
		n.version = curVersion
		n.value = curVal
		return true
	}
	return false
}
