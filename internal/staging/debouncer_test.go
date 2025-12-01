package staging

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/mock"
)

func TestDebouncer(t *testing.T) {
	t.Run("callback is called only once during timeout period", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			mockObj := NewMockedCallbackObj()
			debouncer := NewDebouncer(100*time.Millisecond, mockObj.Callback)
			debouncer.Start(ctx)

			for range 5 {
				debouncer.Debounce()
				time.Sleep(20 * time.Millisecond)
			}

			time.Sleep(1 * time.Second)
			synctest.Wait()

			mockObj.AssertNumberOfCalls(t, "Callback", 1)
		})
	})

	t.Run("multiple callbacks called when requests sent over timeout", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			mockObj := NewMockedCallbackObj()
			debouncer := NewDebouncer(100*time.Millisecond, mockObj.Callback)
			debouncer.Start(ctx)

			for range 10 {
				debouncer.Debounce()
				time.Sleep(20 * time.Millisecond)
			}

			time.Sleep(1 * time.Second)
			synctest.Wait()

			mockObj.AssertNumberOfCalls(t, "Callback", 2)
		})
	})
}

func NewMockedCallbackObj() *MockedCallbackObj {
	mockObj := &MockedCallbackObj{}
	mockObj.On("Callback").Return()
	return mockObj
}

type MockedCallbackObj struct {
	mock.Mock
}

func (m *MockedCallbackObj) Callback() {
	m.Called()
}
