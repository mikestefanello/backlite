package ui

import (
	"testing"
)

func TestMustParse_Panic(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Error("MustParse() did not panic")
		}
	}()
	mustParse("abc")
}
