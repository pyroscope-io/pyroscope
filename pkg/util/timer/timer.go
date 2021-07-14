package timer

import (
	"runtime/debug"
	"time"

	"github.com/sirupsen/logrus"
)

func doFuncWithRecover(name string, fn func() error) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("panic recovered: %v; %v", r, string(debug.Stack()))
		}
	}()

	// do the function
	if err := fn(); err != nil {
		logrus.Errorf("run worker(%s): %v", name, err)
	}
}

// StartWorker news a timer for doing the function
func StartWorker(name string, done chan struct{}, interval time.Duration, fn func() error) {
	go func() {
		// new a timer for doing the function periodly
		ticker := time.NewTimer(interval)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				select {
				case <-done:
					return
				default:
				}

				doFuncWithRecover(name, fn)

				// reset the timer
				ticker.Reset(interval)
			}
		}
	}()
}
