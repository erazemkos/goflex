//go:build e2e

package devserver

import "testing"

func TestStatePersistenceAcrossReloadE2E(t *testing.T) {
	t.Skip("browser state-persistence E2E is reserved for the dedicated chromedp lane")
}
