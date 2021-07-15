// +build phpspy

// Package phpspy is a wrapper around this library called phpspy written in Rust
package phpspy

// #cgo darwin LDFLAGS: -L../../../third_party/phpspy -lphpspy
// #cgo linux LDFLAGS: -L../../../third_party/phpspy -lphpspy
// #include "../../../third_party/phpspy/phpspy.h"
import "C"

import (
	"bytes"
	"errors"
	"time"
	"unsafe"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
)

// TODO: make this configurable
// TODO: pass lower level structures between go and rust?
var bufferLength = 1024 * 64

type PhpSpy struct {
	dataBuf []byte
	dataPtr unsafe.Pointer

	errorBuf []byte
	errorPtr unsafe.Pointer

	pid int
}

func Start(pid int, _ spy.ProfileType, _ uint32, _ bool) (spy.Spy, error) {
	dataBuf := make([]byte, bufferLength)
	dataPtr := unsafe.Pointer(&dataBuf[0])

	errorBuf := make([]byte, bufferLength)
	errorPtr := unsafe.Pointer(&errorBuf[0])

	// Sometimes phpspy doesn't initialize properly right after the process starts so we need to give it some time
	// TODO: handle this better
	time.Sleep(1 * time.Second)

	r := C.phpspy_init(C.int(pid), errorPtr, C.int(bufferLength))

	if r < 0 {
		return nil, errors.New(string(errorBuf[:-r]))
	}

	return &PhpSpy{
		dataPtr:  dataPtr,
		dataBuf:  dataBuf,
		errorBuf: errorBuf,
		errorPtr: errorPtr,
		pid:      pid,
	}, nil
}

func (s *PhpSpy) Stop() error {
	r := C.phpspy_cleanup(C.int(s.pid), s.errorPtr, C.int(bufferLength))
	if r < 0 {
		return errors.New(string(s.errorBuf[:-r]))
	}
	return nil
}

// Snapshot calls callback function with stack-trace or error.
func (s *PhpSpy) Snapshot(cb func([]byte, uint64, error)) {
	r := C.phpspy_snapshot(C.int(s.pid), s.dataPtr, C.int(bufferLength), s.errorPtr, C.int(bufferLength))
	if r < 0 {
		cb(nil, 0, errors.New(string(s.errorBuf[:-r])))
	} else {
		cb(trimSemicolon(s.dataBuf[:r]), 1, nil)
	}
}

func init() {
	spy.RegisterSpy("phpspy", Start)
}

func trimSemicolon(b []byte) []byte {
	if bytes.HasSuffix(b, []byte(";")) {
		return b[:len(b)-1]
	}
	return b
}
