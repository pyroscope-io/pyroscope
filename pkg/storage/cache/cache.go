package cache

import (
	"errors"
	"fmt"
	"sync"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgrijalva/lfu-go"

	"github.com/pyroscope-io/pyroscope/pkg/util/metrics"
)

type Cache struct {
	db     *badger.DB
	lfu    *lfu.Cache
	prefix string

	alwaysSave    bool
	evictionsDone chan struct{}
	flushOnce     sync.Once

	hitCounter          string
	missCounter         string
	storageReadCounter  string
	storageWriteCounter string

	// Bytes serializes objects before they go into storage. Users are required to define this one
	Bytes func(k string, v interface{}) ([]byte, error)
	// FromBytes deserializes object coming from storage. Users are required to define this one
	FromBytes func(k string, v []byte) (interface{}, error)
	// New creates a new object when there's no object in cache or storage. Optional
	New func(k string) interface{}
}

func New(db *badger.DB, prefix, humanReadableName string) *Cache {
	evictionChannel := make(chan lfu.Eviction)
	writeBackChannel := make(chan lfu.Eviction)

	l := lfu.New()

	// eviction channel for saving cache items to disk
	l.EvictionChannel = evictionChannel
	l.WriteBackChannel = writeBackChannel

	// disable the eviction based on upper and lower bound
	l.UpperBound = 0
	l.LowerBound = 0

	cache := &Cache{
		db:  db,
		lfu: l,

		prefix:        prefix,
		evictionsDone: make(chan struct{}),

		hitCounter:          "cache_" + humanReadableName + "_hit",
		missCounter:         "cache_" + humanReadableName + "_miss",
		storageReadCounter:  "storage_" + humanReadableName + "_read",
		storageWriteCounter: "storage_" + humanReadableName + "_write",
	}

	// start a goroutine for saving the evicted cache items to disk

	go func() {
		for {
			e, ok := <-evictionChannel
			if !ok {
				break
			}
			cache.saveToDisk(e.Key, e.Value)
		}
		cache.evictionsDone <- struct{}{}
	}()

	// start a goroutine for saving the evicted cache items to disk
	go func() {
		for {
			e, ok := <-writeBackChannel
			if !ok {
				break
			}
			cache.saveToDisk(e.Key, e.Value)
		}
	}()

	return cache
}

func (cache *Cache) Put(key string, val interface{}) {
	cache.lfu.Set(key, val)
	if cache.alwaysSave {
		cache.saveToDisk(key, val)
	}
}

func (cache *Cache) saveToDisk(key string, val interface{}) error {
	// serialize the key and value
	buf, err := cache.Bytes(key, val)
	if err != nil {
		return fmt.Errorf("serialize key and value: %v", err)
	}

	metrics.Count(cache.storageWriteCounter, 1)
	// update the kv to badger
	if err := cache.db.Update(func(txn *badger.Txn) error {
		return txn.SetEntry(badger.NewEntry([]byte(cache.prefix+key), buf))
	}); err != nil {
		return fmt.Errorf("save to disk: %v", err)
	}
	return nil
}

func (cache *Cache) Flush() {
	cache.flushOnce.Do(func() {
		// evict all the items in cache
		cache.lfu.Evict(cache.lfu.Len())

		close(cache.lfu.EvictionChannel)
		close(cache.lfu.WriteBackChannel)

		// wait until all evictions are done
		<-cache.evictionsDone
	})
}

// Evict performs cache evictions. The difference between Evict and WriteBack is that evictions happen when cache grows
// above allowed threshold and write-back calls happen constantly, making pyroscope more crash-resilient.
// See https://github.com/pyroscope-io/pyroscope/issues/210 for more context
func (cache *Cache) Evict(percent float64) {
	cache.lfu.Evict(int(float64(cache.lfu.Len()) * percent))
}

// See Evict for more information on this
func (cache *Cache) WriteBack() {
	cache.lfu.WriteBack(cache.lfu.Len())
}

func (cache *Cache) Delete(key string) error {
	cache.lfu.Delete(key)

	err := cache.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(cache.prefix + key))
	})

	return err
}

func (cache *Cache) GetOrCreate(key string) (interface{}, error) {
	v, err := cache.lookup(key) // find the key from cache first
	if err != nil {
		return nil, err
	}
	if v != nil {
		return v, nil
	}
	if cache.New == nil {
		return nil, errors.New("cache's New function is nil")
	}
	v = cache.New(key)
	cache.lfu.Set(key, v)
	return v, nil
}

func (cache *Cache) Lookup(key string) (interface{}, bool) {
	v, err := cache.lookup(key)
	if v == nil || err != nil {
		return nil, false
	}
	return v, true
}

func (cache *Cache) lookup(key string) (interface{}, error) {
	// find the key from cache first
	val := cache.lfu.Get(key)
	if val != nil {
		metrics.Count(cache.hitCounter, 1)
		return val, nil
	}
	// logrus.WithField("key", key).Debug("lfu miss")
	metrics.Count(cache.missCounter, 1)

	var copied []byte
	// read the value from badger
	if err := cache.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(cache.prefix + key))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil
			}

			return fmt.Errorf("read from badger: %v", err)
		}

		if err := item.Value(func(val []byte) error {
			copied = append([]byte{}, val...)
			return nil
		}); err != nil {
			return fmt.Errorf("retrieve value from item: %v", err)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("badger view: %v", err)
	}

	// if it's not found from badger, create a new object
	if copied == nil {
		// logrus.WithField("key", key).Debug("storage miss")
		return nil, nil
	}

	// deserialize the object from storage
	metrics.Count(cache.storageReadCounter, 1)
	val, err := cache.FromBytes(key, copied)
	if err != nil {
		return nil, fmt.Errorf("deserialize the object: %v", err)
	}
	cache.lfu.Set(key, val)
	// if it needs to save to disk
	if cache.alwaysSave {
		cache.saveToDisk(key, val)
	}

	// logrus.WithField("key", key).Debug("storage hit")
	return val, nil
}

func (cache *Cache) Size() uint64 {
	return uint64(cache.lfu.Len())
}

func (cache *Cache) Len() int {
	return cache.lfu.Len()
}
