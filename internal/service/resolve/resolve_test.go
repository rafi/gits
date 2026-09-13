package resolve

import (
	"context"
	"errors"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/service"
)

// TestParse covers the one piece of command-line grammar: a trailing "/" on
// the second argument means sub-project, anything else means repository.
func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want Args
	}{
		{"nothing", nil, Args{}},
		{"project", []string{"acme"}, Args{Project: "acme"}},
		{"repo", []string{"acme", "api"}, Args{Project: "acme", Repo: "api"}},
		{"sub-project", []string{"acme", "libs/"}, Args{Project: "acme", Sub: "libs/"}},
	}
	for _, tc := range tests {
		if got := Parse(tc.args, false); got != tc.want {
			t.Errorf("Parse(%q) = %+v, want %+v", tc.args, got, tc.want)
		}
	}
	if got := Parse(nil, true); !got.RequireRepo {
		t.Errorf("Parse(nil, true).RequireRepo = false, want true")
	}
}

// fakeSelector answers with fixed names, recording that it was asked.
type fakeSelector struct {
	project, repo string
	err           error
	askedProject  bool
	askedRepo     bool
}

func (f *fakeSelector) Project(context.Context) (string, error) {
	f.askedProject = true
	return f.project, f.err
}

func (f *fakeSelector) Repo(context.Context, domain.Project, string) (string, error) {
	f.askedRepo = true
	return f.repo, f.err
}

func runtime(t *testing.T) service.Runtime {
	t.Helper()
	return service.Runtime{
		Ctx: t.Context(),
		Projects: domain.ProjectListKeyed{
			"acme": {Repos: []domain.Repository{{Name: "api"}, {Name: "web"}}},
		},
	}
}

func TestResolve(t *testing.T) {
	t.Parallel()

	t.Run("named project and repo never ask", func(t *testing.T) {
		t.Parallel()

		sel := &fakeSelector{}
		p, repo, err := Resolve(Parse([]string{"acme", "api"}, true), sel, runtime(t))
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if p.Name != "acme" || repo == nil || repo.Name != "api" {
			t.Errorf("Resolve = (%q, %+v), want (acme, api)", p.Name, repo)
		}
		if sel.askedProject || sel.askedRepo {
			t.Error("Resolve prompted although both names were given")
		}
	})

	t.Run("no repo wanted, none returned", func(t *testing.T) {
		t.Parallel()

		sel := &fakeSelector{}
		_, repo, err := Resolve(Parse([]string{"acme"}, false), sel, runtime(t))
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if repo != nil || sel.askedRepo {
			t.Errorf("Resolve = %+v, asked=%v, want no repository and no prompt", repo, sel.askedRepo)
		}
	})

	t.Run("a required repo is asked for", func(t *testing.T) {
		t.Parallel()

		sel := &fakeSelector{repo: "web"}
		_, repo, err := Resolve(Parse([]string{"acme"}, true), sel, runtime(t))
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if repo == nil || repo.Name != "web" {
			t.Errorf("Resolve repo = %+v, want web", repo)
		}
	})

	t.Run("declining the picker is a warning", func(t *testing.T) {
		t.Parallel()

		_, _, err := Resolve(Args{}, &fakeSelector{}, runtime(t))
		if !domain.IsWarning(err) {
			t.Errorf("Resolve error = %v, want a downgradeable warning", err)
		}
	})

	t.Run("a named project that does not exist is a hard error", func(t *testing.T) {
		t.Parallel()

		_, _, err := Resolve(Parse([]string{"ghost"}, false), &fakeSelector{}, runtime(t))
		if err == nil || domain.IsWarning(err) {
			t.Errorf("Resolve error = %v, want a real error", err)
		}
	})

	t.Run("Strict refuses instead of prompting", func(t *testing.T) {
		t.Parallel()

		_, _, err := Resolve(Args{}, Strict{}, runtime(t))
		if !errors.Is(err, ErrNoSelection) {
			t.Errorf("Resolve error = %v, want ErrNoSelection", err)
		}
	})
}
