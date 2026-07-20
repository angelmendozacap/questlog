package shared

import "testing"

func TestOK(t *testing.T) {
	got := OK()
	if got.Status != "ok" {
		t.Fatalf("Status = %q, want %q", got.Status, "ok")
	}
}
