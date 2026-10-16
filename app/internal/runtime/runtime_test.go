package runtime

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRestartClearsPreviousDrainMarker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "draining")
	if err := os.WriteFile(path, []byte("old preStop"), 0600); err != nil {
		t.Fatal(err)
	}
	state, err := NewState(path)
	if err != nil {
		t.Fatal(err)
	}
	if state.Draining() {
		t.Fatal("stale marker must not poison restarted container")
	}
	state.Drain()
	if !state.Draining() {
		t.Fatal("draining state was not set")
	}
}
func TestRejectUnboundedOrInvalidRPCDeadline(t *testing.T) {
	for _, value := range []string{"0s", "-1s", "11s", "invalid"} {
		t.Setenv("GRPC_TIMEOUT", value)
		if _, err := Duration("GRPC_TIMEOUT", time.Second); err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
}
