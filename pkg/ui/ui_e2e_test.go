//go:build e2e

package ui_test

import "testing"

func TestBrowserSmokeCounter(t *testing.T) {
	t.Skip("browser E2E for the UI DSL is reserved for the dedicated e2e lane once chromedp scaffolding is enabled")
}
