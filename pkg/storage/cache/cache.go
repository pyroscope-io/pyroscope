package cache

import (
	"errors"
	"fmt"
	"sync"

	"github.com/dgrijalva/lfu-go"
	"github.com/pyroscope-io/pyroscope/pkg/storage/kv"
	"github.com/pyroscope-io/pyroscope/pkg/util/metrics"
	"github.com/sirupsen/logrus"
)

type Cache struct {
	db     kv.Storage
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

func New(db kv.Storage, prefix, humanReadableName string) *Cache {
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

	// start a goroutine for saving the write-back cache items to disk
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
	// update the kv to db
	if err := cache.db.Set([]byte(cache.prefix+key), buf); err != nil {
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

	return cache.db.Del([]byte(cache.prefix + key))
}

func (cache *Cache) Get(key string) (interface{}, error) {
	// find the key from cache first
	val := cache.lfu.Get(key)
	if val != nil {
		metrics.Count(cache.hitCounter, 1)
		return val, nil
	}
	logrus.WithField("key", key).Debug("lfu miss")
	metrics.Count(cache.missCounter, 1)

	// read the value from db
	copied, err := cache.db.Get([]byte(cache.prefix + key))
	if err != nil {
		return nil, fmt.Errorf("read key: %v", err)
	}

	// if it's not found from badger, create a new object
	if copied == nil {
		logrus.WithField("key", key).Debug("storage miss")

		if cache.New == nil {
			return nil, errors.New("cache's New function is nil")
		}

		newVal := cache.New(key)
		cache.lfu.Set(key, newVal)
		return newVal, nil
	}

	// deserialize the object from storage
	metrics.Count(cache.storageReadCounter, 1)
	val, err = cache.FromBytes(key, copied)
	if err != nil {
		return nil, fmt.Errorf("deserialize the object: %v", err)
	}
	cache.lfu.Set(key, val)
	// if it needs to save to disk
	if cache.alwaysSave {
		cache.saveToDisk(key, val)
	}

	logrus.WithField("key", key).Debug("storage hit")
	return val, nil
}

func (cache *Cache) Size() uint64 {
	return uint64(cache.lfu.Len())
}

func (cache *Cache) Len() int {
	return cache.lfu.Len()
}
