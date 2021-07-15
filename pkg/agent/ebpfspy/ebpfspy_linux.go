// +build ebpfspy

// Package ebpfspy provides integration with Linux eBPF. It calls profile.py from BCC tools:
//   https://github.com/iovisor/bcc/blob/master/tools/profile.py
// TODO: At some point we might extract the part that starts another process because it has good potential to be reused by similar profiling tools.
package ebpfspy

import (
	"sync"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
)

type EbpfSpy struct {
	resetMutex sync.Mutex
	reset      bool
	stop       bool

	profilingSession *session

	stopCh chan struct{}
}

func Start(pid int, _ spy.ProfileType, _ uint32, _ bool) (spy.Spy, error) {
	s := newSession(pid)
	err := s.Start()
	if err != nil {
		return nil, err
	}
	return &EbpfSpy{
		profilingSession: s,
		stopCh:           make(chan struct{}),
	}, nil
}

func (s *EbpfSpy) Stop() error {
	s.stop = true
	<-s.stopCh
	return nil
}

func (s *EbpfSpy) Snapshot(cb func([]byte, uint64, error)) {
	s.resetMutex.Lock()
	defer s.resetMutex.Unlock()

	if !s.reset {
		return
	}

	s.reset = false
	s.profilingSession.Reset(func(name []byte, v uint64) {
		cb(name, v, nil)
	})
	if s.stop {
		s.stopCh <- struct{}{}
		s.profilingSession.Stop()
	}
}

func (s *EbpfSpy) Reset() {
	s.resetMutex.Lock()
	defer s.resetMutex.Unlock()

	s.reset = true
}

func init() {
	spy.RegisterSpy("ebpfspy", Start)
}
