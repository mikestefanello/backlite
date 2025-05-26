package backlite

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestQueue_CannotDecode(t *testing.T) {
	q := NewQueue[testTask](func(_ context.Context, _ testTask) error {
		return nil
	})
	err := q.Process(context.Background(), []byte{1, 2, 3})
	if err == nil {
		t.Error("Process should have failed")
	}
}

func TestQueues_GetUnregisteredQueuePanics(t *testing.T) {
	s := &queues{}

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Should be panicking, but it didn't")
		} else {
			if msg, ok := r.(string); !ok || !strings.Contains(msg, fmt.Sprintf("queue '%s' not registered", testTask{}.Config().Name)) {
				t.Errorf("Unexpected panic value: %v", r)
			}
		}
	}()

	s.get(testTask{}.Config().Name)
}
