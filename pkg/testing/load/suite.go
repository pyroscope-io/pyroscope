package load

import (
	"runtime"
	"sync"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

type StorageWriteSuite struct {
	apps    []*App
	sources int

	interval time.Duration
	period   time.Duration
	from     time.Time

	seed    int
	writers int
	writeFn func(*storage.PutInput)
}

type StorageWriteSuiteConfig struct {
	Sources  int
	Interval time.Duration
	Period   time.Duration
	From     time.Time

	Seed    int
	Writers int
	WriteFn func(*storage.PutInput)
}

const (
	defaultInterval = 10 * time.Second
	defaultRandSeed = 23061912
)

var defaultWriters = runtime.NumCPU()

func NewStorageWriteSuite(c StorageWriteSuiteConfig) *StorageWriteSuite {
	s := StorageWriteSuite{
		sources:  c.Sources,
		period:   c.Period,
		from:     c.From,
		writeFn:  c.WriteFn,
		interval: defaultInterval,
		seed:     defaultRandSeed,
		writers:  defaultWriters,
	}
	if s.writeFn == nil {
		panic("WriteFn is required")
	}
	if s.period == 0 {
		panic("Period duration is required")
	}
	if s.sources == 0 {
		panic("Number of sources is required")
	}
	if s.from.IsZero() {
		s.from = time.Now().Add(-s.period)
	}
	if c.Interval > 0 {
		s.interval = c.Interval
	}
	if c.Seed > 0 {
		s.seed = c.Seed
	}
	if c.Writers > 0 {
		s.writers = c.Writers
	}
	return &s
}

func (s *StorageWriteSuite) AddApp(name string, c AppConfig) *StorageWriteSuite {
	s.apps = append(s.apps, NewApp(s.seed, name, c))
	return s
}

type Stats struct {
	RemainingPeriod time.Duration
}

func (s *StorageWriteSuite) Stats() Stats {
	return Stats{
		RemainingPeriod: s.period,
	}
}

func (s *StorageWriteSuite) Start() {
	q := make(chan *storage.PutInput)
	wg := new(sync.WaitGroup)
	wg.Add(s.writers)
	for i := 0; i < s.writers; i++ {
		go func() {
			defer wg.Done()
			for p := range q {
				s.writeFn(p)
			}
		}()
	}
	from := s.from
	for s.period > 0 {
		to := from.Add(s.interval)
		for i := 0; i < s.sources; i++ {
			a := s.apps[i%len(s.apps)]
			q <- a.CreatePutInput(from, to)
		}
		from = to
		s.period -= s.interval
	}
	close(q)
	wg.Wait()
}
