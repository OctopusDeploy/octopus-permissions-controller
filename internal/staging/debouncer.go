package staging

import (
	"context"
	"time"
)

type Debouncer struct {
	timeout  time.Duration
	callback func()
	timeChan chan time.Time
}

func NewDebouncer(timeout time.Duration, callback func()) *Debouncer {
	return &Debouncer{
		timeout:  timeout,
		callback: callback,
		timeChan: make(chan time.Time, 100),
	}
}

func (m *Debouncer) Start(ctx context.Context) {
	go func() {
		var startedTime *time.Time

		ticker := time.NewTicker(m.timeout)
		ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ticker.Stop()
				startedTime = nil
				m.callback()

			case timestamp := <-m.timeChan:
				if startedTime == nil {
					startedTime = &timestamp
					ticker.Reset(m.timeout)
				}
			}
		}
	}()
}

func (m *Debouncer) Debounce() {
	m.timeChan <- time.Now()
}
