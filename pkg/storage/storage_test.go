package storage

import (
	"os"
	"runtime"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
	"github.com/shirou/gopsutil/mem"
	"github.com/sirupsen/logrus"
	stats "gopkg.in/alexcesaro/statsd.v2"
)

// 21:22:08      air |  (time.Duration) 16m40s,
// 21:22:08      air |  (time.Duration) 2h46m40s,
// 21:22:08      air |  (time.Duration) 27h46m40s,
// 21:22:08      air |  (time.Duration) 277h46m40s,
// 21:22:08      air |  (time.Duration) 2777h46m40s,
// 21:22:08      air |  (time.Duration) 27777h46m40s

var (
	s      *Storage
	client *stats.Client
)

var _ = BeforeSuite(func() {
	statAddr := os.Getenv("STORAGE_TEST_STATS_ADDR")
	if statAddr != "" {
		cli, err := stats.New(stats.Address(statAddr), stats.Prefix("storage_test"))
		Expect(err).ToNot(HaveOccurred())
		client = cli
	}
})

var _ = Describe("storage package", func() {
	logrus.SetLevel(logrus.InfoLevel)

	testing.WithConfig(func(cfg **config.Config) {
		JustBeforeEach(func() {
			evictInterval = 2 * time.Second

			var err error
			s, err = New(&(*cfg).Server)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("delete tests", func() {
			Context("simple delete", func() {
				It("works correctly", func() {
					tree := tree.New()
					tree.Insert([]byte("a;b"), uint64(1))
					tree.Insert([]byte("a;c"), uint64(2))
					st := testing.SimpleTime(10)
					et := testing.SimpleTime(19)
					st2 := testing.SimpleTime(0)
					et2 := testing.SimpleTime(30)
					key, _ := ParseKey("foo")

					s.Put(&PutInput{
						StartTime:  st,
						EndTime:    et,
						Key:        key,
						Val:        tree,
						SpyName:    "testspy",
						SampleRate: 100,
					})

					err := s.Delete(&DeleteInput{
						StartTime: st,
						EndTime:   et,
						Key:       key,
					})
					Expect(err).ToNot(HaveOccurred())

					gOut, err := s.Get(&GetInput{
						StartTime: st2,
						EndTime:   et2,
						Key:       key,
					})

					Expect(err).ToNot(HaveOccurred())
					Expect(gOut).To(BeNil())
					Expect(s.Close()).ToNot(HaveOccurred())
				})
			})
			Context("delete all trees", func() {
				It("works correctly", func() {
					tree1 := tree.New()
					tree1.Insert([]byte("a;b"), uint64(1))
					tree1.Insert([]byte("a;c"), uint64(2))
					tree2 := tree.New()
					tree2.Insert([]byte("c;d"), uint64(1))
					tree2.Insert([]byte("e;f"), uint64(2))
					st := testing.SimpleTime(10)
					et := testing.SimpleTime(19)
					st2 := testing.SimpleTime(0)
					et2 := testing.SimpleTime(30)
					key, _ := ParseKey("foo")

					s.Put(&PutInput{
						StartTime:  st,
						EndTime:    et,
						Key:        key,
						Val:        tree1,
						SpyName:    "testspy",
						SampleRate: 100,
					})

					s.Put(&PutInput{
						StartTime:  st,
						EndTime:    et,
						Key:        key,
						Val:        tree2,
						SpyName:    "testspy",
						SampleRate: 100,
					})

					err := s.Delete(&DeleteInput{
						StartTime: st,
						EndTime:   et,
						Key:       key,
					})
					Expect(err).ToNot(HaveOccurred())

					gOut, err := s.Get(&GetInput{
						StartTime: st2,
						EndTime:   et2,
						Key:       key,
					})
					Expect(err).ToNot(HaveOccurred())
					Expect(gOut).To(BeNil())
					Expect(s.Close()).ToNot(HaveOccurred())
				})
			})
			Context("put after delete", func() {
				It("works correctly", func() {
					tree1 := tree.New()
					tree1.Insert([]byte("a;b"), uint64(1))
					tree1.Insert([]byte("a;c"), uint64(2))
					tree2 := tree.New()
					tree2.Insert([]byte("c;d"), uint64(1))
					tree2.Insert([]byte("e;f"), uint64(2))
					st := testing.SimpleTime(10)
					et := testing.SimpleTime(19)
					st2 := testing.SimpleTime(0)
					et2 := testing.SimpleTime(30)
					key, _ := ParseKey("foo")

					err := s.Put(&PutInput{
						StartTime:  st,
						EndTime:    et,
						Key:        key,
						Val:        tree1,
						SpyName:    "testspy",
						SampleRate: 100,
					})
					Expect(err).ToNot(HaveOccurred())

					err = s.Delete(&DeleteInput{
						StartTime: st,
						EndTime:   et,
						Key:       key,
					})
					Expect(err).ToNot(HaveOccurred())

					s.Put(&PutInput{
						StartTime:  st,
						EndTime:    et,
						Key:        key,
						Val:        tree2,
						SpyName:    "testspy",
						SampleRate: 100,
					})

					gOut, err := s.Get(&GetInput{
						StartTime: st2,
						EndTime:   et2,
						Key:       key,
					})

					Expect(err).ToNot(HaveOccurred())
					Expect(gOut.Tree).ToNot(BeNil())
					Expect(gOut.Tree.String()).To(Equal(tree2.String()))
					Expect(s.Close()).ToNot(HaveOccurred())
				})
			})
		})

		Context("smoke tests", func() {
			Context("check segment cache", func() {
				It("works correctly", func() {
					tree := tree.New()

					size := 32
					treeKey := make([]byte, size)
					for i := 0; i < size; i++ {
						treeKey[i] = 'a'
					}
					for i := 0; i < 60; i++ {
						k := string(treeKey) + strconv.Itoa(i+1)
						tree.Insert([]byte(k), uint64(i+1))

						key, _ := ParseKey("tree key" + strconv.Itoa(i+1))
						err := s.Put(&PutInput{
							Key:        key,
							Val:        tree,
							SpyName:    "testspy",
							SampleRate: 100,
						})
						Expect(err).ToNot(HaveOccurred())
					}
					Expect(s.Close()).ToNot(HaveOccurred())
				})
			})
			Context("simple 10 second write", func() {
				It("works correctly", func() {
					tree := tree.New()
					tree.Insert([]byte("a;b"), uint64(1))
					tree.Insert([]byte("a;c"), uint64(2))
					st := testing.SimpleTime(10)
					et := testing.SimpleTime(19)
					st2 := testing.SimpleTime(0)
					et2 := testing.SimpleTime(30)
					key, _ := ParseKey("foo")

					err := s.Put(&PutInput{
						StartTime:  st,
						EndTime:    et,
						Key:        key,
						Val:        tree,
						SpyName:    "testspy",
						SampleRate: 100,
					})
					Expect(err).ToNot(HaveOccurred())

					o, err := s.Get(&GetInput{
						StartTime: st2,
						EndTime:   et2,
						Key:       key,
					})

					Expect(err).ToNot(HaveOccurred())
					Expect(o.Tree).ToNot(BeNil())
					Expect(o.Tree.String()).To(Equal(tree.String()))
					Expect(s.Close()).ToNot(HaveOccurred())
				})
			})
			Context("simple 20 second write", func() {
				It("works correctly", func() {
					tree := tree.New()
					tree.Insert([]byte("a;b"), uint64(2))
					tree.Insert([]byte("a;c"), uint64(4))
					st := testing.SimpleTime(10)
					et := testing.SimpleTime(29)
					st2 := testing.SimpleTime(0)
					et2 := testing.SimpleTime(30)
					key, _ := ParseKey("foo")

					err := s.Put(&PutInput{
						StartTime:  st,
						EndTime:    et,
						Key:        key,
						Val:        tree,
						SpyName:    "testspy",
						SampleRate: 100,
					})
					Expect(err).ToNot(HaveOccurred())

					o, err := s.Get(&GetInput{
						StartTime: st2,
						EndTime:   et2,
						Key:       key,
					})

					Expect(err).ToNot(HaveOccurred())
					Expect(o.Tree).ToNot(BeNil())
					Expect(o.Tree.String()).To(Equal(tree.String()))
					Expect(s.Close()).ToNot(HaveOccurred())
				})
			})
			Context("evict cache items periodly", func() {
				It("works correctly", func() {
					tree := tree.New()

					size := 16
					treeKey := make([]byte, size)
					for i := 0; i < size; i++ {
						treeKey[i] = 'a'
					}
					for i := 0; i < 200; i++ {
						k := string(treeKey) + strconv.Itoa(i+1)
						tree.Insert([]byte(k), uint64(i+1))

						key, _ := ParseKey("tree key" + strconv.Itoa(i+1))
						err := s.Put(&PutInput{
							Key:        key,
							Val:        tree,
							SpyName:    "testspy",
							SampleRate: 100,
						})
						Expect(err).ToNot(HaveOccurred())
					}

					for i := 0; i < 5; i++ {
						if client != nil {
							vm, err := mem.VirtualMemory()
							Expect(err).ToNot(HaveOccurred())
							client.Gauge("Total", vm.Total)

							var m runtime.MemStats
							runtime.ReadMemStats(&m)
							client.Gauge("NumGC", m.NumGC)
							client.Gauge("Alloc", m.Alloc)
							client.Gauge("Used", float64(m.Alloc)/float64(vm.Total))
							client.Gauge("Segments", s.segments.Len())
						} else {
							logrus.Infof("segments: %v", s.segments.Len())
						}
						time.Sleep(evictInterval)
					}
				})
			})
			Context("persist data between restarts", func() {
				It("works correctly", func() {
					tree := tree.New()
					tree.Insert([]byte("a;b"), uint64(1))
					tree.Insert([]byte("a;c"), uint64(2))
					st := testing.SimpleTime(10)
					et := testing.SimpleTime(19)
					st2 := testing.SimpleTime(0)
					et2 := testing.SimpleTime(30)
					key, _ := ParseKey("foo")

					err := s.Put(&PutInput{
						StartTime:  st,
						EndTime:    et,
						Key:        key,
						Val:        tree,
						SpyName:    "testspy",
						SampleRate: 100,
					})
					Expect(err).ToNot(HaveOccurred())

					o, err := s.Get(&GetInput{
						StartTime: st2,
						EndTime:   et2,
						Key:       key,
					})

					Expect(err).ToNot(HaveOccurred())
					Expect(o.Tree).ToNot(BeNil())
					Expect(o.Tree.String()).To(Equal(tree.String()))
					Expect(s.Close()).ToNot(HaveOccurred())

					s2, err := New(&(*cfg).Server)
					Expect(err).ToNot(HaveOccurred())

					o2, err := s2.Get(&GetInput{
						StartTime: st2,
						EndTime:   et2,
						Key:       key,
					})
					Expect(err).ToNot(HaveOccurred())
					Expect(o2.Tree).ToNot(BeNil())
					Expect(o2.Tree.String()).To(Equal(tree.String()))
					Expect(s2.Close()).ToNot(HaveOccurred())
				})
			})
		})
	})
})
