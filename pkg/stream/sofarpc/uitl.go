package sofarpc

import (
	"sync/atomic"
	"time"
)

// thread-safe reusable timer
type timer struct {
	callback   func()
	interval   time.Duration
	innerTimer *time.Timer
	stopped    int32
	started    int32
	stopChan   chan bool
}

func newTimer(callback func()) *timer {
	return &timer{
		callback: callback,
		stopChan: make(chan bool, 1),
	}
}

func (t *timer) start(interval time.Duration) {
	if !atomic.CompareAndSwapInt32(&t.started, 0, 1) {
		return
	}

	if t.innerTimer == nil {
		t.innerTimer = time.NewTimer(interval)
	} else {
		t.innerTimer.Reset(interval)
	}

	go func() {
		defer func() {
			t.innerTimer.Stop()
			atomic.StoreInt32(&t.started, 0)
			atomic.StoreInt32(&t.stopped, 0)
		}()

		select {
		case <-t.innerTimer.C:
			t.callback()
		case <-t.stopChan:
			return
		}
	}()
}

func (t *timer) stop() {
	if !atomic.CompareAndSwapInt32(&t.stopped, 0, 1) {
		return
	}

	t.stopChan <- true
}

func (t *timer) close() {
	close(t.stopChan)
}
