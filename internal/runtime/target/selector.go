package target

import (
	"context"
	"errors"

	"github.com/rafi/gits/domain"
)

// ErrNoSelection is what a Selector returns when it cannot make a choice at
// all — a client with no terminal to prompt on.
var ErrNoSelection = errors.New("nothing selected")

// Selector supplies the names the arguments did not. It deals in names rather
// than domain objects, so an implementation cannot hand back a repository that
// belongs to another project.
//
// An empty name with a nil error means the user was asked and declined; that
// is a warning, not a failure.
type Selector interface {
	Project(ctx context.Context) (string, error)
	// Repo picks a repository within p. root names the project a preview
	// should re-invoke gits with, which differs from p.Name only when p is a
	// sub-project.
	Repo(ctx context.Context, p domain.Project, root string) (string, error)
}

// Strict answers every question with ErrNoSelection. It is the Selector for a
// caller that cannot prompt: an HTTP request, a test.
type Strict struct{}

// Project refuses: there is nobody to ask.
func (Strict) Project(context.Context) (string, error) { return "", ErrNoSelection }

// Repo refuses: there is nobody to ask.
func (Strict) Repo(context.Context, domain.Project, string) (string, error) {
	return "", ErrNoSelection
}
