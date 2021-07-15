package storage

import (
	"runtime"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/util/metrics"
)

func (s *Storage) periodicTask(interval time.Duration, cb func()) {
	timer := time.NewTimer(interval)
	defer func() {
		timer.Stop()
		s.wg.Done()
	}()
	for {
		select {
		case <-s.stop:
			return
		case <-timer.C:
			select {
			case <-s.stop:
				return
			default:
				cb()
				timer.Reset(interval)
			}
		}
	}
}

func (*Storage) badgerGCTask(db *badger.DB) func() {
	return func() {
		logrus.Debug("starting badger garbage collection")
		for {
			if err := db.RunValueLogGC(0.7); err != nil {
				return
			}
		}
	}
}

func (s *Storage) evictionTask(memTotal uint64) func() {
	var m runtime.MemStats
	return func() {
		runtime.ReadMemStats(&m)
		used := float64(m.Alloc) / float64(memTotal)
		metrics.Gauge("evictions_alloc_bytes", m.Alloc)
		metrics.Gauge("evictions_total_bytes", memTotal)
		metrics.Gauge("evictions_used_perc", used)

		percent := s.config.CacheEvictVolume
		if used > s.config.CacheEvictThreshold {
			metrics.Timing("evictions_timer", func() {
				metrics.Count("evictions_count", 1)
				s.dimensions.Evict(percent / 4)
				s.dicts.Evict(percent / 4)
				s.segments.Evict(percent / 2)
				s.trees.Evict(percent)
			})
		}
	}
}

func (s *Storage) writeBackTask() {
	metrics.Timing("write_back_timer", func() {
		metrics.Count("write_back_count", 1)
		s.dimensions.WriteBack()
		s.segments.WriteBack()
		s.dicts.WriteBack()
		s.trees.WriteBack()
	})
}

func (s *Storage) retentionTask() {
	logrus.Debug("starting retention task")
	metrics.Timing("retention_timer", func() {
		metrics.Count("retention_count", 1)
		if err := s.DeleteDataBefore(s.lifetimeBasedRetentionThreshold()); err != nil {
			logrus.WithError(err).Warn("retention task failed")
		}
	})
}
