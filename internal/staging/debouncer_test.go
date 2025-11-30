package staging

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	clocktesting "k8s.io/utils/clock/testing"
)

func TestDebouncer(t *testing.T) {
	t.Run("callback is called only once during timeout period", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		callbackCh := make(chan struct{}, 10)
		fakeClock := clocktesting.NewFakeClock(time.Now())
		mockObj := NewMockedCallbackObj(callbackCh)
		debouncer := NewDebouncer(100*time.Millisecond, mockObj.Callback, fakeClock)
		debouncer.Start(ctx)

		for range 5 {
			debouncer.Debounce()
			waitForWaiters(fakeClock, 1)
			fakeClock.Step(20 * time.Millisecond)
		}

		waitForWaiters(fakeClock, 1)
		fakeClock.Step(100 * time.Millisecond)
		<-callbackCh

		assert.True(t, mockObj.AssertNumberOfCalls(t, "Callback", 1))
	})

	t.Run("multiple callbacks called when requests sent over timeout", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		callbackCh := make(chan struct{}, 10)
		fakeClock := clocktesting.NewFakeClock(time.Now())
		mockObj := NewMockedCallbackObj(callbackCh)
		debouncer := NewDebouncer(100*time.Millisecond, mockObj.Callback, fakeClock)
		debouncer.Start(ctx)

		for i := range 10 {
			debouncer.Debounce()
			waitForWaiters(fakeClock, 1)
			fakeClock.Step(20 * time.Millisecond)

			if i == 4 {
				fakeClock.Step(100 * time.Millisecond)
				<-callbackCh
			}
		}

		waitForWaiters(fakeClock, 1)
		fakeClock.Step(100 * time.Millisecond)
		<-callbackCh

		assert.True(t, mockObj.AssertNumberOfCalls(t, "Callback", 2))
	})
}

// nolint:unparam // count is always 1 in current tests, but keeping it for possible future tests
func waitForWaiters(fakeClock *clocktesting.FakeClock, count int) {
	for range 100 {
		if fakeClock.HasWaiters() && fakeClock.Waiters() >= count {
			return
		}
		time.Sleep(1 * time.Millisecond)
	}
}

func NewMockedCallbackObj(callbackCh chan struct{}) *MockedCallbackObj {
	mockObj := &MockedCallbackObj{}
	mockObj.On("Callback").Return().Run(func(args mock.Arguments) {
		callbackCh <- struct{}{}
	})
	return mockObj
}

type MockedCallbackObj struct {
	mock.Mock
}

func (m *MockedCallbackObj) Callback() {
	m.Called()
}
