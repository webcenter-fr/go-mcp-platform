package storetest

import (
	"testing"

	"github.com/webcenter-fr/go-mcp-platform/internal/approval/store"
)

// TestConformanceSuiteRunsAgainstMemory guards the helper package itself: the
// suite must compile and pass against a known-good store.
func TestConformanceSuiteRunsAgainstMemory(t *testing.T) {
	Run(t, func(t *testing.T) store.ApprovalStore {
		return store.NewMemory()
	})
}
