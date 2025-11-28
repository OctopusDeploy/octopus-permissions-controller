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
				log.V(1).Info("Debouncer timeout reached, invoking callback")
				callback()

			case timestamp := <-d.timeChan:
				if startedTime == nil {
					log.V(1).Info("Debouncer received event", "timestamp", timestamp)
					startedTime = &timestamp
					ticker.Reset(timeout)
				} else {
					log.V(1).Info("Debouncer ignored event", "timestamp", timestamp)
				}
			}
		}
	}()

	return d
}

func (m *Debouncer) Debounce() {
	m.timeChan <- time.Now()
}
