package browse

import (
	"testing"

	"github.com/rafi/gits/domain"
)

// TestBrowseTarget covers the argument dispatch of ExecBrowse: the project
// name must survive every arg count, and an explicit branch is only taken
// from a 3-arg invocation.
func TestBrowseTarget(t *testing.T) {
	project := domain.Project{Name: "selected"}
	tests := []struct {
		name       string
		args       []string
		wantProj   string
		wantBranch string
	}{
		{"no args uses selected project", []string{}, "selected", ""},
		{"project only", []string{"acme"}, "acme", ""},
		{"project and repo", []string{"acme", "repo"}, "acme", ""},
		{"project repo branch", []string{"acme", "repo", "main"}, "acme", "main"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotProj, gotBranch := browseTarget(tt.args, project)
			if gotProj != tt.wantProj {
				t.Errorf("projName = %q, want %q", gotProj, tt.wantProj)
			}
			if gotBranch != tt.wantBranch {
				t.Errorf("branch = %q, want %q", gotBranch, tt.wantBranch)
			}
		})
	}
}
