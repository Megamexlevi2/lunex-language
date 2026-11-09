package runtime

import "testing"

func TestNativeRuntimeStatusIsConsistent(t *testing.T) {
	status := GetNativeRuntimeStatus()
	if status.Effective != (status.Enabled && status.Available) {
		t.Fatalf("effective state does not match enabled/available state: %+v", status)
	}
	if status.HotThreshold < 1 {
		t.Fatalf("hot threshold must be positive, got %d", status.HotThreshold)
	}
	if !status.Available && status.UnavailableReason == "" {
		t.Fatal("unavailable native JIT must include a reason")
	}
	if status.Available && status.UnavailableReason != "" {
		t.Fatalf("available native JIT must not include an unavailable reason: %q", status.UnavailableReason)
	}
}
