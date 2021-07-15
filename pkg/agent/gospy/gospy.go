// +build !nogospy

package gospy

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"

	custom_pprof "github.com/pyroscope-io/pyroscope/pkg/agent/pprof"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/convert"
)

// TODO: make this configurable
// TODO: pass lower level structures between go and rust?
var bufferLength = 1024 * 64

type GoSpy struct {
	resetMutex    sync.Mutex
	reset         bool
	stop          bool
	profileType   spy.ProfileType
	disableGCRuns bool
	sampleRate    uint32

	lastGCGeneration uint32

	stopCh chan struct{}
	buf    *bytes.Buffer
}

func startCPUProfile(w io.Writer, hz uint32) error {
	// idea here is that for most people we're starting the default profiler
	//   but if you want to use a different sampling rate we use our experimental profiler
	if hz == 100 {
		return pprof.StartCPUProfile(w)
	}
	return custom_pprof.StartCPUProfile(w, hz)
}

func Start(_ int, profileType spy.ProfileType, sampleRate uint32, disableGCRuns bool) (spy.Spy, error) {
	s := &GoSpy{
		stopCh:        make(chan struct{}),
		buf:           &bytes.Buffer{},
		profileType:   profileType,
		disableGCRuns: disableGCRuns,
		sampleRate:    sampleRate,
	}
	if s.profileType == spy.ProfileCPU {
		if err := startCPUProfile(s.buf, sampleRate); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *GoSpy) Stop() error {
	s.stop = true
	<-s.stopCh
	return nil
}

// TODO: this is not the most elegant solution as it creates global state
//   the idea here is that we can reuse heap profiles
var (
	lastProfileMutex     sync.Mutex
	lastProfile          *convert.Profile
	lastProfileCreatedAt time.Time
)

func getHeapProfile(b *bytes.Buffer) *convert.Profile {
	lastProfileMutex.Lock()
	defer lastProfileMutex.Unlock()

	if lastProfile == nil || !lastProfileCreatedAt.After(time.Now().Add(-1*time.Second)) {
		pprof.WriteHeapProfile(b)
		g, _ := gzip.NewReader(bytes.NewReader(b.Bytes()))

		lastProfile, _ = convert.ParsePprof(g)
		lastProfileCreatedAt = time.Now()
	}

	return lastProfile
}

func numGC() uint32 {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	return memStats.NumGC
}

// Snapshot calls callback function with stack-trace or error.
func (s *GoSpy) Snapshot(cb func([]byte, uint64, error)) {
	s.resetMutex.Lock()
	defer s.resetMutex.Unlock()

	// before the upload rate is reached, no need to read the profile data
	if !s.reset {
		return
	}
	s.reset = false

	if s.profileType == spy.ProfileCPU {
		// stop the previous cycle of sample collection
		pprof.StopCPUProfile()
		defer func() {
			// start a new cycle of sample collection
			if err := startCPUProfile(s.buf, s.sampleRate); err != nil {
				cb(nil, uint64(0), err)
			}
		}()

		// new gzip reader with the read data in buffer
		r, err := gzip.NewReader(bytes.NewReader(s.buf.Bytes()))
		if err != nil {
			cb(nil, uint64(0), fmt.Errorf("new gzip reader: %v", err))
			return
		}

		// parse the read data with pprof format
		profile, err := convert.ParsePprof(r)
		if err != nil {
			cb(nil, uint64(0), fmt.Errorf("parse pprof: %v", err))
			return
		}
		profile.Get("samples", func(name []byte, val int) {
			cb(name, uint64(val), nil)
		})
	} else {
		// this is current GC generation
		currentGCGeneration := numGC()

		// sometimes GC doesn't run within 10 seconds
		//   in such cases we force a GC run
		//   users can disable it with disableGCRuns option
		if currentGCGeneration == s.lastGCGeneration && !s.disableGCRuns {
			runtime.GC()
			currentGCGeneration = numGC()
		}

		// if there's no GC run then the profile is gonna be the same
		//   in such case it does not make sense to upload the same profile twice
		if currentGCGeneration != s.lastGCGeneration {
			getHeapProfile(s.buf).Get(string(s.profileType), func(name []byte, val int) {
				cb(name, uint64(val), nil)
			})
			s.lastGCGeneration = currentGCGeneration
		}
	}
	s.buf.Reset()
}

func (s *GoSpy) Reset() {
	s.resetMutex.Lock()
	defer s.resetMutex.Unlock()

	s.reset = true
}
