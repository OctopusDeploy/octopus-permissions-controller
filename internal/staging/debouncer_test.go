package staging

import (
	"context"
	"testing"
	"time"
)

func TestDebouncer(t *testing.T) {
	t.Run("callback is called only once during timeout period", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		called := make(chan struct{}, 1)
		debouncer := NewDebouncer(ctx, 100*time.Millisecond, func() {
			called <- struct{}{}
		})

		go func() {
			for range 5 {
				debouncer.Debounce()
				time.Sleep(20 * time.Millisecond)
			}
		}()

		AssertCallbackCalled(t, called)

		// Ensure no additional calls are made
		AssertCallbackNotCalled(t, called)
	})

	t.Run("multiple callbacks called when requests sent over timeout", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		called := make(chan struct{}, 1)
		debouncer := NewDebouncer(ctx, 100*time.Millisecond, func() {
			called <- struct{}{}
		})

		go func() {
			for range 10 {
				debouncer.Debounce()
				time.Sleep(20 * time.Millisecond)
			}
		}()

		AssertCallbackCalled(t, called)
		AssertCallbackCalled(t, called)

		// Ensure no additional calls are made
		AssertCallbackNotCalled(t, called)
	})
}

func AssertCallbackCalled(t *testing.T, called chan struct{}) {
	select {
	case <-called:
		// Success
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Debouncer callback was not called in expected time")
	}
}

func AssertCallbackNotCalled(t *testing.T, called chan struct{}) {
	select {
	case <-called:
		t.Fatal("Debouncer callback was called more than once")
	case <-time.After(500 * time.Millisecond):
		// Success
	}
}
