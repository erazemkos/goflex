package version

import "testing"

func TestVersionDefaultAndInjected(t *testing.T) {
	t.Cleanup(setForTest(""))
	if got := Version(); got != "dev" {
		t.Fatalf("Version()=%q want dev", got)
	}
	restore := setForTest("v1.2.3")
	defer restore()
	if got := Version(); got != "v1.2.3" {
		t.Fatalf("Version()=%q", got)
	}
}
