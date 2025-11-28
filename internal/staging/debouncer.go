package staging

import (
	"context"
	"time"
)

type Debouncer struct {
	timeChan chan time.Time
}

func NewDebouncer(ctx context.Context, timeout time.Duration, callback func()) *Debouncer {
	d := &Debouncer{
		timeChan: make(chan time.Time, 100),
	}

	go func() {
		var startedTime *time.Time

		ticker := time.NewTicker(timeout)
		ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ticker.Stop()
				startedTime = nil
				callback()

			case timestamp := <-d.timeChan:
				if startedTime == nil {
					startedTime = &timestamp
					ticker.Reset(timeout)
				}
			}
		}
	}()

	return d
}

func (m *Debouncer) Debounce() {
	m.timeChan <- time.Now()
}
