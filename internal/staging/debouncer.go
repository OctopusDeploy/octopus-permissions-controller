package staging

import (
	"context"
	"time"

	"k8s.io/utils/clock"
)

type Debouncer struct {
	timeout  time.Duration
	callback func()
	timeChan chan time.Time
	clock    clock.Clock
}

func NewDebouncer(timeout time.Duration, callback func(), clk clock.Clock) *Debouncer {
	return &Debouncer{
		timeout:  timeout,
		callback: callback,
		timeChan: make(chan time.Time, 100),
		clock:    clk,
	}
}

func (m *Debouncer) Start(ctx context.Context) {
	go func() {
		var startedTime *time.Time

		timer := m.clock.NewTimer(m.timeout)
		timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C():
				timer.Stop()
				startedTime = nil
				m.callback()

			case timestamp := <-m.timeChan:
				if startedTime == nil {
					startedTime = &timestamp
					timer.Reset(m.timeout)
				}
			}
		}
	}()
}

func (m *Debouncer) Debounce() {
	m.timeChan <- m.clock.Now()
}
