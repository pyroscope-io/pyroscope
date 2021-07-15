package storage

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/pyroql"
	"github.com/pyroscope-io/pyroscope/pkg/storage/cache"
	"github.com/pyroscope-io/pyroscope/pkg/storage/dict"
	"github.com/pyroscope-io/pyroscope/pkg/storage/dimension"
	"github.com/pyroscope-io/pyroscope/pkg/storage/labels"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/merge"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
	"github.com/pyroscope-io/pyroscope/pkg/util/disk"
	"github.com/pyroscope-io/pyroscope/pkg/util/metrics"
	"github.com/pyroscope-io/pyroscope/pkg/util/slices"
)

var (
	errOutOfSpace = errors.New("running out of space")
	errRetention  = errors.New("could not write because of retention settings")

	evictInterval     = time.Second
	writeBackInterval = time.Second
	retentionInterval = time.Minute
	gcInterval        = 5 * time.Minute
)

type Storage struct {
	putMutex sync.Mutex

	config   *config.Server
	segments *cache.Cache

	dimensions *cache.Cache
	dicts      *cache.Cache
	trees      *cache.Cache
	labels     *labels.Labels

	db           *badger.DB
	dbTrees      *badger.DB
	dbDicts      *badger.DB
	dbDimensions *badger.DB
	dbSegments   *badger.DB

	localProfilesDir string

	stop chan struct{}
	wg   sync.WaitGroup
}

func (s *Storage) newBadger(name string) (*badger.DB, error) {
	badgerPath := filepath.Join(s.config.StoragePath, name)
	err := os.MkdirAll(badgerPath, 0o755)
	if err != nil {
		return nil, err
	}
	badgerOptions := badger.DefaultOptions(badgerPath)
	badgerOptions = badgerOptions.WithTruncate(!s.config.BadgerNoTruncate)
	badgerOptions = badgerOptions.WithSyncWrites(false)
	badgerOptions = badgerOptions.WithCompactL0OnClose(false)
	badgerOptions = badgerOptions.WithCompression(options.ZSTD)
	badgerLevel := logrus.ErrorLevel
	if l, err := logrus.ParseLevel(s.config.BadgerLogLevel); err == nil {
		badgerLevel = l
	}
	badgerOptions = badgerOptions.WithLogger(badgerLogger{name: name, logLevel: badgerLevel})
	db, err := badger.Open(badgerOptions)
	if err != nil {
		return nil, err
	}
	s.wg.Add(1)
	go s.periodicTask(gcInterval, s.badgerGCTask(db))
	return db, nil
}

func New(c *config.Server) (*Storage, error) {
	s := &Storage{
		config:           c,
		stop:             make(chan struct{}),
		localProfilesDir: filepath.Join(c.StoragePath, "local-profiles"),
	}
	var err error
	s.db, err = s.newBadger("main")
	if err != nil {
		return nil, err
	}
	s.labels = labels.New(s.db)
	s.dbTrees, err = s.newBadger("trees")
	if err != nil {
		return nil, err
	}
	s.dbDicts, err = s.newBadger("dicts")
	if err != nil {
		return nil, err
	}
	s.dbDimensions, err = s.newBadger("dimensions")
	if err != nil {
		return nil, err
	}
	s.dbSegments, err = s.newBadger("segments")
	if err != nil {
		return nil, err
	}

	if err = os.MkdirAll(s.localProfilesDir, 0o755); err != nil {
		return nil, err
	}

	s.dimensions = cache.New(s.dbDimensions, "i:", "dimensions")
	s.dimensions.Bytes = func(k string, v interface{}) ([]byte, error) {
		return v.(*dimension.Dimension).Bytes()
	}
	s.dimensions.FromBytes = func(k string, v []byte) (interface{}, error) {
		return dimension.FromBytes(v)
	}
	s.dimensions.New = func(k string) interface{} {
		return dimension.New()
	}

	s.segments = cache.New(s.dbSegments, "s:", "segments")
	s.segments.Bytes = func(k string, v interface{}) ([]byte, error) {
		return v.(*segment.Segment).Bytes()
	}
	s.segments.FromBytes = func(k string, v []byte) (interface{}, error) {
		// TODO:
		//   these configuration params should be saved in db when it initializes
		return segment.FromBytes(v)
	}
	s.segments.New = func(k string) interface{} {
		return segment.New()
	}

	s.dicts = cache.New(s.dbDicts, "d:", "dicts")
	s.dicts.Bytes = func(k string, v interface{}) ([]byte, error) {
		return v.(*dict.Dict).Bytes()
	}
	s.dicts.FromBytes = func(k string, v []byte) (interface{}, error) {
		return dict.FromBytes(v)
	}
	s.dicts.New = func(k string) interface{} {
		return dict.New()
	}

	s.trees = cache.New(s.dbTrees, "t:", "trees")
	s.trees.Bytes = s.treeBytes
	s.trees.FromBytes = s.treeFromBytes
	s.trees.New = func(k string) interface{} {
		return tree.New()
	}

	memTotal, err := getMemTotal()
	if err != nil {
		return nil, err
	}

	s.wg.Add(2)
	go s.periodicTask(evictInterval, s.evictionTask(memTotal))
	go s.periodicTask(writeBackInterval, s.writeBackTask)
	if s.config.Retention > 0 {
		s.wg.Add(1)
		go s.periodicTask(retentionInterval, s.retentionTask)
	}

	return s, nil
}

type PutInput struct {
	StartTime       time.Time
	EndTime         time.Time
	Key             *Key
	Val             *tree.Tree
	SpyName         string
	SampleRate      uint32
	Units           string
	AggregationType string
}

func (s *Storage) treeFromBytes(k string, v []byte) (interface{}, error) {
	key := fromTreeToDictKey(k)
	d, ok := s.dicts.Lookup(key)
	if !ok {
		// The key not found. Fallback to segment key form which has been
		// used before tags support. Refer to fromTreeToDictKey.
		return s.treeFromBytesFallback(k, v)
	}
	return tree.FromBytes(d.(*dict.Dict), v)
}

func (s *Storage) treeFromBytesFallback(k string, v []byte) (interface{}, error) {
	key := fromTreeToMainKey(k)
	d, ok := s.dicts.Lookup(key)
	if !ok {
		return nil, nil
	}
	return tree.FromBytes(d.(*dict.Dict), v)
}

func (s *Storage) treeBytes(k string, v interface{}) ([]byte, error) {
	key := fromTreeToDictKey(k)
	d, err := s.dicts.GetOrCreate(key)
	if err != nil {
		return nil, fmt.Errorf("dicts cache for %v: %v", key, err)
	}
	b, err := v.(*tree.Tree).Bytes(d.(*dict.Dict), s.config.MaxNodesSerialization)
	if err != nil {
		return nil, fmt.Errorf("dicts cache for %v: %v", key, err)
	}
	s.dicts.Put(key, d)
	return b, nil
}

var OutOfSpaceThreshold = 512 * bytesize.MB

func (s *Storage) Put(po *PutInput) error {
	// TODO: This is a pretty broad lock. We should find a way to make these locks more selective.
	s.putMutex.Lock()
	defer s.putMutex.Unlock()

	if err := s.performFreeSpaceCheck(); err != nil {
		return err
	}

	if po.StartTime.Before(s.lifetimeBasedRetentionThreshold()) {
		return errRetention
	}

	logrus.WithFields(logrus.Fields{
		"startTime":       po.StartTime.String(),
		"endTime":         po.EndTime.String(),
		"key":             po.Key.Normalized(),
		"samples":         po.Val.Samples(),
		"units":           po.Units,
		"aggregationType": po.AggregationType,
	}).Debug("storage.Put")

	for k, v := range po.Key.labels {
		s.labels.Put(k, v)
	}

	sk := po.Key.SegmentKey()
	for k, v := range po.Key.labels {
		key := k + ":" + v
		r, err := s.dimensions.GetOrCreate(key)
		if err != nil {
			logrus.Errorf("dimensions cache for %v: %v", key, err)
			continue
		}
		r.(*dimension.Dimension).Insert([]byte(sk))
		s.dimensions.Put(key, r)
	}

	r, err := s.segments.GetOrCreate(sk)
	if err != nil {
		return fmt.Errorf("segments cache for %v: %v", sk, err)
	}

	st := r.(*segment.Segment)
	st.SetMetadata(po.SpyName, po.SampleRate, po.Units, po.AggregationType)
	samples := po.Val.Samples()

	st.Put(po.StartTime, po.EndTime, samples, func(depth int, t time.Time, r *big.Rat, addons []segment.Addon) {
		tk := po.Key.TreeKey(depth, t)
		res, err := s.trees.GetOrCreate(tk)
		if err != nil {
			logrus.Errorf("trees cache for %v: %v", tk, err)
			return
		}
		cachedTree := res.(*tree.Tree)
		treeClone := po.Val.Clone(r)
		for _, addon := range addons {
			if res, ok := s.trees.Lookup(po.Key.TreeKey(addon.Depth, addon.T)); ok {
				ta := res.(*tree.Tree)
				ta.RLock()
				treeClone.Merge(ta)
				ta.RUnlock()
			}
		}
		cachedTree.Lock()
		cachedTree.Merge(treeClone)
		cachedTree.Unlock()
		s.trees.Put(tk, cachedTree)
	})

	s.segments.Put(sk, st)
	return nil
}

type GetInput struct {
	StartTime time.Time
	EndTime   time.Time
	Key       *Key
	Query     *pyroql.Query
}

type GetOutput struct {
	Tree       *tree.Tree
	Timeline   *segment.Timeline
	SpyName    string
	SampleRate uint32
	Units      string
}

func (s *Storage) Get(gi *GetInput) (*GetOutput, error) {
	logger := logrus.WithFields(logrus.Fields{
		"startTime": gi.StartTime.String(),
		"endTime":   gi.EndTime.String(),
	})

	var dimensionKeys func() []dimension.Key
	switch {
	case gi.Key != nil:
		logger = logger.WithField("key", gi.Key.Normalized())
		dimensionKeys = s.dimensionKeysByKey(gi.Key)
	case gi.Query != nil:
		logger = logger.WithField("query", gi.Query)
		dimensionKeys = s.dimensionKeysByQuery(gi.Query)
	default:
		return nil, fmt.Errorf("key or query must be specified")
	}

	logger.Trace("storage.Get")
	var (
		triesToMerge []merge.Merger
		lastSegment  *segment.Segment
		writesTotal  uint64

		timeline        = segment.GenerateTimeline(gi.StartTime, gi.EndTime)
		aggregationType = "sum"
	)

	for _, k := range dimensionKeys() {
		// TODO: refactor, store `Key`s in dimensions
		parsedKey, err := ParseKey(string(k))
		if err != nil {
			logrus.Errorf("parse key: %v: %v", string(k), err)
			continue
		}
		key := parsedKey.SegmentKey()
		res, ok := s.segments.Lookup(key)
		if !ok {
			continue
		}

		st := res.(*segment.Segment)
		if st.AggregationType() == "average" {
			aggregationType = "average"
		}

		timeline.PopulateTimeline(st)
		lastSegment = st

		st.Get(gi.StartTime, gi.EndTime, func(depth int, samples, writes uint64, t time.Time, r *big.Rat) {
			if res, ok = s.trees.Lookup(parsedKey.TreeKey(depth, t)); ok {
				triesToMerge = append(triesToMerge, res.(*tree.Tree).Clone(r))
				writesTotal += writes
			}
		})
	}

	resultTrie := merge.MergeTriesSerially(runtime.NumCPU(), triesToMerge...)
	if resultTrie == nil {
		return nil, nil
	}

	t := resultTrie.(*tree.Tree)
	if writesTotal > 0 && aggregationType == "average" {
		t = t.Clone(big.NewRat(1, int64(writesTotal)))
	}

	return &GetOutput{
		Tree:       t,
		Timeline:   timeline,
		SpyName:    lastSegment.SpyName(),
		SampleRate: lastSegment.SampleRate(),
		Units:      lastSegment.Units(),
	}, nil
}

func (s *Storage) dimensionKeysByKey(key *Key) func() []dimension.Key {
	return func() []dimension.Key {
		var dimensions []*dimension.Dimension
		for k, v := range key.labels {
			if d, ok := s.lookupDimensionKV(k, v); ok {
				dimensions = append(dimensions, d)
			}
		}
		return dimension.Intersection(dimensions...)
	}
}

func (s *Storage) dimensionKeysByQuery(qry *pyroql.Query) func() []dimension.Key {
	return func() []dimension.Key { return s.exec(context.TODO(), qry) }
}

func (s *Storage) iterateOverAllSegments(cb func(*Key, *segment.Segment) error) error {
	nameKey := "__name__"

	var dimensions []*dimension.Dimension
	s.labels.GetValues(nameKey, func(v string) bool {
		dmInt, ok := s.dimensions.Lookup(nameKey + ":" + v)
		if !ok {
			return true
		}
		dimensions = append(dimensions, dmInt.(*dimension.Dimension))
		return true
	})

	for _, rawSk := range dimension.Union(dimensions...) {
		sk, _ := ParseKey(string(rawSk))
		stInt, ok := s.segments.Lookup(sk.SegmentKey())
		if !ok {
			continue
		}
		st := stInt.(*segment.Segment)
		if err := cb(sk, st); err != nil {
			return err
		}
	}
	return nil
}

func (s *Storage) DeleteDataBefore(threshold time.Time) error {
	return s.iterateOverAllSegments(func(sk *Key, st *segment.Segment) error {
		var err error
		deletedRoot := st.DeleteDataBefore(threshold, func(depth int, t time.Time) {
			tk := sk.TreeKey(depth, t)
			if delErr := s.trees.Delete(tk); delErr != nil {
				err = delErr
			}
		})
		if err != nil {
			return err
		}

		if deletedRoot {
			s.deleteSegmentAndRelatedData(sk)
		}
		return nil
	})
}

type DeleteInput struct {
	Key *Key
}

var maxTime = time.Unix(1<<62, 999999999)

func (s *Storage) Delete(di *DeleteInput) error {
	var dimensions []*dimension.Dimension
	for k, v := range di.Key.labels {
		dInt, ok := s.dimensions.Lookup(k + ":" + v)
		if !ok {
			return nil
		}
		dimensions = append(dimensions, dInt.(*dimension.Dimension))
	}

	for _, sk := range dimension.Intersection(dimensions...) {
		skk, _ := ParseKey(string(sk))
		stInt, ok := s.segments.Lookup(skk.SegmentKey())
		if !ok {
			continue
		}
		st := stInt.(*segment.Segment)
		var err error
		st.Get(zeroTime, maxTime, func(depth int, _, _ uint64, t time.Time, _ *big.Rat) {
			treeKey := skk.TreeKey(depth, t)
			err = s.trees.Delete(treeKey)
		})
		if err != nil {
			return err
		}

		s.deleteSegmentAndRelatedData(skk)
	}

	return nil
}

func (s *Storage) deleteSegmentAndRelatedData(key *Key) error {
	s.dicts.Delete(key.DictKey())
	s.segments.Delete(key.SegmentKey())
	for k, v := range key.labels {
		dInt, ok := s.dimensions.Lookup(k + ":" + v)
		if !ok {
			continue
		}
		d := dInt.(*dimension.Dimension)
		d.Delete(dimension.Key(key.SegmentKey()))
	}
	return nil
}

func (s *Storage) Close() error {
	close(s.stop)
	s.wg.Wait()

	metrics.Timing("storage_caches_flush_timer", func() {
		wg := sync.WaitGroup{}
		wg.Add(3)
		go func() { defer wg.Done(); s.dimensions.Flush() }()
		go func() { defer wg.Done(); s.segments.Flush() }()
		go func() { defer wg.Done(); s.trees.Flush() }()
		wg.Wait()

		// dictionary has to flush last because trees write to dictionaries
		s.dicts.Flush()
	})

	metrics.Timing("storage_badger_close_timer", func() {
		wg := sync.WaitGroup{}
		wg.Add(5)
		go func() { defer wg.Done(); s.dbTrees.Close() }()
		go func() { defer wg.Done(); s.dbDicts.Close() }()
		go func() { defer wg.Done(); s.dbDimensions.Close() }()
		go func() { defer wg.Done(); s.dbSegments.Close() }()
		go func() { defer wg.Done(); s.db.Close() }()
		wg.Wait()
	})
	// this allows prometheus to collect metrics before pyroscope exits
	if os.Getenv("PYROSCOPE_WAIT_AFTER_STOP") != "" {
		time.Sleep(5 * time.Second)
	}
	return nil
}

func (s *Storage) GetKeys(cb func(_k string) bool) {
	s.labels.GetKeys(cb)
}

func (s *Storage) GetValues(key string, cb func(v string) bool) {
	s.labels.GetValues(key, func(v string) bool {
		if key != "__name__" || !slices.StringContains(s.config.HideApplications, v) {
			return cb(v)
		}
		return true
	})
}

func (s *Storage) DiskUsage() map[string]bytesize.ByteSize {
	res := map[string]bytesize.ByteSize{
		"main":       0,
		"trees":      0,
		"dicts":      0,
		"dimensions": 0,
		"segments":   0,
	}
	for k := range res {
		res[k] = dirSize(filepath.Join(s.config.StoragePath, k))
	}
	return res
}

func dirSize(path string) (result bytesize.ByteSize) {
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			result += bytesize.ByteSize(info.Size())
		}
		return nil
	})
	return result
}

func (s *Storage) CacheStats() map[string]interface{} {
	return map[string]interface{}{
		"dimensions_size": s.dimensions.Size(),
		"segments_size":   s.segments.Size(),
		"dicts_size":      s.dicts.Size(),
		"trees_size":      s.trees.Size(),
	}
}

var zeroTime time.Time

func (s *Storage) lifetimeBasedRetentionThreshold() time.Time {
	var t time.Time
	if s.config.Retention != 0 {
		t = time.Now().Add(-1 * s.config.Retention)
	}
	return t
}

func (s *Storage) performFreeSpaceCheck() error {
	freeSpace, err := disk.FreeSpace(s.config.StoragePath)
	if err == nil {
		if freeSpace < OutOfSpaceThreshold {
			return errOutOfSpace
		}
	}
	return nil
}
