package backlite

import (
	"context"
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
