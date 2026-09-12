package sync

import (
	"errors"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/types"
)

// stubCache is a no-op Cacher.
type stubCache struct{}

func (stubCache) Get(string, *domain.Project) (bool, error) { return false, nil }
func (stubCache) Save(string, domain.Project) error         { return nil }
func (stubCache) Flush(domain.Project) error                { return nil }

// TestExecSyncUnknownProject proves syncing a non-existent project warns
// instead of silently exiting zero.
func TestExecSyncUnknownProject(t *testing.T) {
	deps := types.RuntimeCLI{Runtime: types.Runtime{
		Projects: domain.ProjectListKeyed{},
		Cache:    stubCache{},
	}}
	err := ExecSync([]string{"bogus"}, deps)
	if err == nil {
		t.Fatal("ExecSync(bogus) = nil, want a warning")
	}
	var warn *types.Warning
	if !errors.As(err, &warn) {
		t.Errorf("ExecSync(bogus) error = %T (%v), want *types.Warning", err, err)
	}
}
