package status

import (
	"errors"
	"time"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/infra/git"
	"github.com/rafi/gits/internal/runtime/command"
	"github.com/rafi/gits/internal/service/status"
)

// RenderSample renders a fixed status table through the real table renderer,
// covering every Status glyph, for `gits theme show`. It touches neither git
// nor the filesystem.
func RenderSample(deps app.RuntimeCLI) {
	now := time.Now()
	head := func(subject string, ago time.Duration) git.Head {
		return git.Head{Hash: "a1b2c3d", Subject: subject, Time: now.Add(-ago)}
	}
	proj := domain.Project{Name: "sample"}
	mk := func(name string, state domain.RepoState, r status.Report) *status.Report {
		r.Repo = command.Repo{
			Repository: domain.Repository{Name: name, State: state},
			Project:    proj, ProjectKey: "sample", Path: name,
		}
		if r.Branch == "" {
			r.Branch = "main"
		}
		return &r
	}
	tracking := func(ahead, behind int) git.Snapshot {
		return git.Snapshot{Branch: "main", Upstream: "origin/main", Tracking: true, Ahead: ahead, Behind: behind}
	}
	reps := []*status.Report{
		mk("clean", domain.RepoStateOK, status.Report{Snapshot: tracking(0, 0), Compared: true, Head: head("chore: tidy", 2*time.Hour)}),
		mk("staged", domain.RepoStateOK, status.Report{Snapshot: withTree(tracking(0, 0), 2, 0, 0), Compared: true, Head: head("feat: add api", 3*24*time.Hour)}),
		mk("unstaged", domain.RepoStateOK, status.Report{Snapshot: withTree(tracking(0, 0), 0, 3, 0), Compared: true, Head: head("fix: typo", 5*time.Minute)}),
		mk("untracked", domain.RepoStateOK, status.Report{Snapshot: withTree(tracking(0, 0), 0, 0, 4), Compared: true, Head: head("docs: readme", 2*7*24*time.Hour)}),
		mk("ahead", domain.RepoStateOK, status.Report{Snapshot: tracking(3, 0), Compared: true, Head: head("feat: ship it", time.Hour)}),
		mk("behind", domain.RepoStateOK, status.Report{Snapshot: tracking(0, 5), Compared: true, Head: head("refactor: split", 40*24*time.Hour)}),
		mk("diverged", domain.RepoStateOK, status.Report{Snapshot: tracking(2, 7), Compared: true, Head: head("wip", 30*time.Minute)}),
		mk("gone", domain.RepoStateOK, status.Report{Snapshot: git.Snapshot{Branch: "old", Upstream: "origin/old"}, Head: head("feat: old branch", 400*24*time.Hour)}),
		mk("broken", domain.RepoStateError, status.Report{Err: errors.New("not a readable git repository")}),
		mk("missing", domain.RepoStateNotCloned, status.Report{Err: errors.New("not cloned")}),
	}
	res := command.Results[*status.Report]{Project: proj}
	for _, r := range reps {
		proj.Repos = append(proj.Repos, r.Repo.Repository)
		res.Results = append(res.Results, command.Result[*status.Report]{Repo: r.Repo, Value: r, Err: r.Err})
	}
	res.Project = proj
	renderTables(res, Options{}, deps)
}

// withTree sets work-tree counts on a snapshot.
func withTree(s git.Snapshot, staged, unstaged, untracked int) git.Snapshot {
	s.Staged, s.Unstaged, s.Untracked = staged, unstaged, untracked
	return s
}
