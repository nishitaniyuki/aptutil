package cacher

import (
	"container/heap"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/cybozu-go/aptutil/apt"
	"github.com/cybozu-go/log"
	"github.com/pkg/errors"
)

const (
	fileSuffix = ".cache"
)

var (
	// ErrNotFound is returned by Storage.Lookup for non-existing items.
	ErrNotFound = errors.New("not found")

	// ErrBadPath is returned by Storage.Insert if path is bad
	ErrBadPath = errors.New("bad path")

	// ErrInvalidData is returned by Storage.Insert if retrieved data is invalid
	ErrInvalidData = errors.New("can not insert invalid data")
)

// entry represents an item in the cache.
type entry struct {
	*apt.FileInfo

	// for container/heap.
	// atime is used as priorities.
	atime uint64
	index int
}

// FilePath returns the filename of the entry.
func (e *entry) FilePath() string {
	return e.Path() + fileSuffix
}

// Storage stores cache items in local file system.
//
// Cached items will be removed in LRU fashion when the total size of
// items exceeds the capacity.
type Storage struct {
	dir      string // directory for cache items
	capacity uint64

	mu     sync.Mutex
	used   uint64
	cache  map[string]*entry
	lru    []*entry // for container/heap
	lclock uint64   // ditto
}

// NewStorage creates a Storage.
//
// dir is the directory for cached items.
// capacity is the maximum total size (bytes) of items in the cache.
// If capacity is zero, items will not be evicted.
func NewStorage(dir string, capacity uint64) *Storage {
	if !filepath.IsAbs(dir) {
		panic("dir must be an absolute path")
	}
	return &Storage{
		dir:      dir,
		cache:    make(map[string]*entry),
		capacity: capacity,
	}
}

// Len implements heap.Interface.
func (cm *Storage) Len() int {
	return len(cm.lru)
}

// Less implements heap.Interface.
func (cm *Storage) Less(i, j int) bool {
	return cm.lru[i].atime < cm.lru[j].atime
}

// Swap implements heap.Interface.
func (cm *Storage) Swap(i, j int) {
	cm.lru[i], cm.lru[j] = cm.lru[j], cm.lru[i]
	cm.lru[i].index = i
	cm.lru[j].index = j
}

// Push implements heap.Interface.
func (cm *Storage) Push(x interface{}) {
	e, ok := x.(*entry)
	if !ok {
		panic("Storage.Push: wrong type")
	}
	n := len(cm.lru)
	e.index = n
	cm.lru = append(cm.lru, e)
}

// Pop implements heap.Interface.
func (cm *Storage) Pop() interface{} {
	n := len(cm.lru)
	e := cm.lru[n-1]
	e.index = -1 // for safety
	cm.lru = cm.lru[0 : n-1]
	return e
}

// maint removes unused items from cache until used < capacity.
// cm.mu lock must be acquired beforehand.
func (cm *Storage) maint() {
	for cm.capacity > 0 && cm.used > cm.capacity {
		e := heap.Pop(cm).(*entry)
		delete(cm.cache, e.Path())
		cm.used -= e.Size()
		if err := os.Remove(filepath.Join(cm.dir, e.FilePath())); err != nil {
			log.Warn("Storage.maint", map[string]interface{}{
				"error": err.Error(),
			})
		}
		log.Info("removed", map[string]interface{}{
			"path": e.Path(),
		})
	}
}

func readData(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return ioutil.ReadAll(f)
}

// Load loads existing items in filesystem.
func (cm *Storage) Load() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	wf := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		subpath, err := filepath.Rel(cm.dir, path)
		if err != nil {
			return err
		}
		if filepath.Ext(subpath) != fileSuffix {
			return nil
		}
		subpath = subpath[:len(subpath)-len(fileSuffix)]
		if _, ok := cm.cache[subpath]; ok {
			return nil
		}

		size := uint64(info.Size())
		e := &entry{
			// delay calculation of checksums.
			FileInfo: apt.MakeFileInfoNoChecksum(subpath, size),
			atime:    cm.lclock,
			index:    len(cm.lru),
		}
		cm.used += size
		cm.lclock++
		cm.lru = append(cm.lru, e)
		cm.cache[subpath] = e
		log.Debug("Storage.Load", map[string]interface{}{
			"path": subpath,
		})
		return nil
	}

	if err := filepath.Walk(cm.dir, wf); err != nil {
		return err
	}
	heap.Init(cm)

	cm.maint()

	return nil
}

// Insert inserts or updates a cache item.
//
// p must be as clean as filepath.Clean() and
// must not be filepath.IsAbs().
func (cm *Storage) Insert(r io.Reader, p string, fi *apt.FileInfo) (*apt.FileInfo, error) {
	switch {
	case p != filepath.Clean(p):
		return nil, ErrBadPath
	case filepath.IsAbs(p):
		return nil, ErrBadPath
	case p == ".":
		return nil, ErrBadPath
	}

	f, err := ioutil.TempFile(cm.dir, "_tmp")
	if err != nil {
		return nil, err
	}
	defer func() {
		f.Close()
		os.Remove(f.Name())
	}()

	fi2, err := apt.CopyWithFileInfo(f, r, p)
	if err != nil {
		return nil, err
	}

	err = f.Sync()
	if err != nil {
		return nil, err
	}

	if fi != nil && fi.HasChecksum() && !fi.Same(fi2) {
		return nil, ErrInvalidData
	}

	destpath := filepath.Join(cm.dir, p+fileSuffix)
	dirpath := filepath.Dir(destpath)

	_, err = os.Stat(dirpath)
	switch {
	case os.IsNotExist(err):
		err = os.MkdirAll(dirpath, 0755)
		if err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	if existing, ok := cm.cache[p]; ok {
		err = os.Remove(destpath)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, err
			}
			log.Warn("cache file was removed already", map[string]interface{}{
				"path": p,
			})
		}
		cm.used -= existing.Size()
		heap.Remove(cm, existing.index)
		delete(cm.cache, p)
		if log.Enabled(log.LvDebug) {
			log.Debug("deleted existing item", map[string]interface{}{
				"path": p,
			})
		}
	}

	err = os.Rename(f.Name(), destpath)
	if err != nil {
		return nil, err
	}

	e := &entry{
		FileInfo: fi2,
		atime:    cm.lclock,
	}
	cm.used += fi2.Size()
	cm.lclock++
	heap.Push(cm, e)
	cm.cache[p] = e

	cm.maint()

	return fi2, nil
}

func calcChecksum(dir string, e *entry) error {
	if e.FileInfo.HasChecksum() {
		return nil
	}

	data, err := readData(filepath.Join(dir, e.FilePath()))
	if err != nil {
		return err
	}
	e.FileInfo.CalcChecksums(data)
	return nil
}

// Lookup looks up an item in the cache.
// If no item matching fi is found, ErrNotFound is returned.
//
// The caller is responsible to close the returned os.File.
func (cm *Storage) Lookup(fi *apt.FileInfo) (*os.File, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	e, ok := cm.cache[fi.Path()]
	if !ok {
		return nil, ErrNotFound
	}

	// delayed checksum calculation
	err := calcChecksum(cm.dir, e)
	if err != nil {
		return nil, err
	}

	if !fi.Same(e.FileInfo) {
		// checksum mismatch
		return nil, ErrNotFound
	}

	e.atime = cm.lclock
	cm.lclock++
	heap.Fix(cm, e.index)
	return os.Open(filepath.Join(cm.dir, e.FilePath()))
}

// ListAll returns a list of *apt.FileInfo for all cached items.
func (cm *Storage) ListAll() []*apt.FileInfo {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	l := make([]*apt.FileInfo, cm.Len())
	for i, e := range cm.lru {
		l[i] = e.FileInfo
	}
	return l
}

// Delete deletes an item from the cache.
func (cm *Storage) Delete(p string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	e, ok := cm.cache[p]
	if !ok {
		return nil
	}

	err := os.Remove(filepath.Join(cm.dir, e.FilePath()))
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		log.Warn("cached file was already removed", map[string]interface{}{
			"path": p,
		})
	}

	cm.used -= e.Size()
	heap.Remove(cm, e.index)
	delete(cm.cache, p)
	log.Info("deleted item", map[string]interface{}{
		"path": p,
	})
	return nil
}
