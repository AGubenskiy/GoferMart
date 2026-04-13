package logger

import "testing"

func TestNewReturnsLogger(t *testing.T) {
	t.Parallel()

	if got := New(); got == nil {
		t.Fatal("New() returned nil logger")
	}
}
