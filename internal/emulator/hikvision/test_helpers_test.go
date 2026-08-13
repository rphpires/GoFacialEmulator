package hikvision

import (
	"os"
	"testing"

	"GoFacialEmulator/internal/trace"
)

// TestMain disables tracer file IO before any test in this package runs.
// trace.NewTracer() is a sync.Once singleton; if it initializes without
// the marker file it opens logs/trace.log under the package working dir.
func TestMain(m *testing.M) {
	_ = os.WriteFile("DisableTrace.txt", []byte(""), 0644)
	defer os.Remove("DisableTrace.txt")
	os.Exit(m.Run())
}

// newTestEmulator returns a minimal *Emulator suitable for handler unit
// tests. Tests that need repo access must set their own fields; this helper
// only guarantees a valid tracer so methods that log do not nil-panic.
func newTestEmulator(t *testing.T) *Emulator {
	t.Helper()
	return &Emulator{
		tracer:   trace.NewTracer(),
		stopChan: make(chan struct{}),
	}
}

func TestNewTestEmulator_Smoke(t *testing.T) {
	e := newTestEmulator(t)
	if e.tracer == nil {
		t.Fatal("expected non-nil tracer")
	}
	e.tracer.Info("smoke-test log line from newTestEmulator")
}
