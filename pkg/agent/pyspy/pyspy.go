// +build pyspy

// Package pyspy is a wrapper around this library called pyspy written in Rust
package pyspy

// #cgo darwin LDFLAGS: -L../../../third_party/rustdeps/target/release -lrustdeps
// #cgo linux LDFLAGS: -L../../../third_party/rustdeps/target/release -lrustdeps
// #include "../../../third_party/rustdeps/pyspy.h"
import "C"

import (
	"errors"
	"time"
	"unsafe"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
)

// TODO: make this configurable
// TODO: pass lower level structures between go and rust?
var bufferLength = 1024 * 64

// TODO: we should probably find a better way of setting this
var Blocking bool

type PySpy struct {
	dataPtr unsafe.Pointer
	dataBuf []byte

	errorBuf []byte
	errorPtr unsafe.Pointer

	pid int
}

func Start(pid int, _ spy.ProfileType, _ uint32, _ bool) (spy.Spy, error) {
	dataBuf := make([]byte, bufferLength)
	dataPtr := unsafe.Pointer(&dataBuf[0])

	errorBuf := make([]byte, bufferLength)
	errorPtr := unsafe.Pointer(&errorBuf[0])

	// Sometimes pyspy doesn't initialize properly right after the process starts so we need to give it some time
	// TODO: handle this better
	time.Sleep(1 * time.Second)

	blocking := 0
	if Blocking {
		blocking = 1
	}
	r := C.pyspy_init(C.int(pid), C.int(blocking), errorPtr, C.int(bufferLength))

	if r < 0 {
		return nil, errors.New(string(errorBuf[:-r]))
	}

	return &PySpy{
		dataPtr:  dataPtr,
		dataBuf:  dataBuf,
		errorBuf: errorBuf,
		errorPtr: errorPtr,
		pid:      pid,
	}, nil
}

func (s *PySpy) Stop() error {
	r := C.pyspy_cleanup(C.int(s.pid), s.errorPtr, C.int(bufferLength))
	if r < 0 {
		return errors.New(string(s.errorBuf[:-r]))
	}
	return nil
}

// Snapshot calls callback function with stack-trace or error.
func (s *PySpy) Snapshot(cb func([]byte, uint64, error)) {
	r := C.pyspy_snapshot(C.int(s.pid), s.dataPtr, C.int(bufferLength), s.errorPtr, C.int(bufferLength))
	if r < 0 {
		cb(nil, 0, errors.New(string(s.errorBuf[:-r])))
	} else {
		cb(s.dataBuf[:r], 1, nil)
	}
}

func init() {
	spy.RegisterSpy("pyspy", Start)
}
