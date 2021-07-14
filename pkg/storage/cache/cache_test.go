package cache

import (
	"fmt"
	"log"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/storage/kv/badger"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("cache", func() {
	It("works properly", func(done Done) {
		tdir := testing.TmpDirSync()
		badgerPath := filepath.Join(tdir.Path)
		db, err := badger.NewService(&badger.Config{
			StoragePath: badgerPath,
		})
		Expect(err).ToNot(HaveOccurred())

		cache := New(db, "prefix:", "test_cache")
		cache.New = func(k string) interface{} {
			return k
		}
		cache.Bytes = func(k string, v interface{}) ([]byte, error) {
			return []byte(v.(string)), nil
		}
		cache.Bytes = func(k string, v interface{}) ([]byte, error) {
			return []byte(v.(string)), nil
		}
		cache.FromBytes = func(k string, v []byte) (interface{}, error) {
			return string(v), nil
		}
		for i := 0; i < 200; i++ {
			cache.Put(fmt.Sprintf("foo-%d", i), fmt.Sprintf("bar-%d", i))
		}
		log.Printf("size: %d", cache.Len())

		v, err := cache.Get("foo-199")
		Expect(err).ToNot(HaveOccurred())
		Expect(v).To(Equal("bar-199"))

		v, err = cache.Get("foo-1")
		Expect(err).ToNot(HaveOccurred())
		Expect(v).To(Equal("bar-1"))

		v, err = cache.Get("foo-1234")
		Expect(err).ToNot(HaveOccurred())
		Expect(v).To(Equal("foo-1234"))
		cache.Flush()

		close(done)
	}, 3)
})
