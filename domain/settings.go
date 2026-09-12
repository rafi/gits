package domain

import (
	"time"

	log "github.com/sirupsen/logrus"
)

// DefaultCacheTTL is the cache lifetime used when settings.cacheTTL is unset or
// unparseable.
const DefaultCacheTTL = 7 * 24 * time.Hour

// DefaultProviderTimeout bounds provider HTTP calls when
// settings.providerTimeout is unset or unparseable.
const DefaultProviderTimeout = 5 * time.Minute

// DefaultGitTimeout bounds network git operations (clone/fetch/pull) when
// settings.gitTimeout is unset or unparseable.
const DefaultGitTimeout = 5 * time.Minute

type Settings struct {
	Cache           *bool  `json:"cache,omitempty"`
	Finder          Finder `json:"finder"`
	GitTimeout      string `json:"gitTimeout,omitempty"`
	Icons           Icons  `json:"icons"`
	Theme           Theme  `json:"theme"`
	CacheTTL        string `json:"cacheTTL,omitempty"`
	IncludeArchived bool   `json:"includeArchived,omitempty"`
	ProviderTimeout string `json:"providerTimeout,omitempty"`
	Verbose         bool   `json:"verbose,omitempty"`
	WorkerCount     int    `json:"workerCount,omitempty"`
}

// ProviderTimeoutDuration returns the parsed providerTimeout setting, or
// DefaultProviderTimeout when it is empty or cannot be parsed as a Go
// duration (e.g. "90s").
func (s Settings) ProviderTimeoutDuration() time.Duration {
	if s.ProviderTimeout == "" {
		return DefaultProviderTimeout
	}
	d, err := time.ParseDuration(s.ProviderTimeout)
	if err != nil {
		log.Warnf("invalid providerTimeout %q, using default: %v", s.ProviderTimeout, err)
		return DefaultProviderTimeout
	}
	return d
}

// GitTimeoutDuration returns the parsed gitTimeout setting, or
// DefaultGitTimeout when it is empty or cannot be parsed as a Go duration
// (e.g. "30m").
func (s Settings) GitTimeoutDuration() time.Duration {
	if s.GitTimeout == "" {
		return DefaultGitTimeout
	}
	d, err := time.ParseDuration(s.GitTimeout)
	if err != nil {
		log.Warnf("invalid gitTimeout %q, using default: %v", s.GitTimeout, err)
		return DefaultGitTimeout
	}
	return d
}

// CacheTTLDuration returns the parsed cacheTTL setting, or DefaultCacheTTL when
// it is empty or cannot be parsed as a Go duration (e.g. "168h").
func (s Settings) CacheTTLDuration() time.Duration {
	if s.CacheTTL == "" {
		return DefaultCacheTTL
	}
	d, err := time.ParseDuration(s.CacheTTL)
	if err != nil {
		log.Warnf("invalid cacheTTL %q, using default: %v", s.CacheTTL, err)
		return DefaultCacheTTL
	}
	return d
}

type Finder struct {
	Binary string   `json:"binary"`
	Args   []string `json:"args,omitempty"`
	Extra  []string `json:"extra,omitempty"`
}

type Icons struct {
	Modified  string `json:"modified,omitempty"`
	Untracked string `json:"untracked,omitempty"`
	Staged    string `json:"staged,omitempty"`
	Unstaged  string `json:"unstaged,omitempty"`
	DiffError string `json:"diffError,omitempty"`
	DiffClean string `json:"diffClean,omitempty"`
	Ahead     string `json:"ahead,omitempty"`
	Behind    string `json:"behind,omitempty"`
	Diverged  string `json:"diverged,omitempty"`
	NA        string `json:"na,omitempty"`
}

// ApplyDefaults fills unset icon fields with the built-in status glyphs, so
// user YAML only needs to override the ones it changes.
func (i *Icons) ApplyDefaults() {
	defaults := Icons{
		Modified:  "≠",
		Untracked: "?",
		Staged:    "+",
		Unstaged:  "!",
		DiffError: "✘",
		DiffClean: "|",
		Ahead:     "⇡",
		Behind:    "⇣",
		Diverged:  "⇅",
		NA:        "–",
	}
	if i.Modified == "" {
		i.Modified = defaults.Modified
	}
	if i.Untracked == "" {
		i.Untracked = defaults.Untracked
	}
	if i.Staged == "" {
		i.Staged = defaults.Staged
	}
	if i.Unstaged == "" {
		i.Unstaged = defaults.Unstaged
	}
	if i.DiffError == "" {
		i.DiffError = defaults.DiffError
	}
	if i.DiffClean == "" {
		i.DiffClean = defaults.DiffClean
	}
	if i.Ahead == "" {
		i.Ahead = defaults.Ahead
	}
	if i.Behind == "" {
		i.Behind = defaults.Behind
	}
	if i.Diverged == "" {
		i.Diverged = defaults.Diverged
	}
	if i.NA == "" {
		i.NA = defaults.NA
	}
}

type Style struct {
	Color string `json:"color,omitempty"`
	Align string `json:"align,omitempty"`
	Width int    `json:"width,omitempty"`
	Bold  bool   `json:"bold,omitempty"`
	Faint bool   `json:"faint,omitempty"`
}

type Theme struct {
	// General
	Normal        Style `json:"normal"`
	Bullet        Style `json:"bullet"`
	PreviewHeader Style `json:"previewHeader"`

	// Project
	ProjectTitle Style `json:"projectTitle"`
	Provider     Style `json:"provider"`
	Desc         Style `json:"desc"`

	// Repository
	RepoTitle Style `json:"repoTitle"`
	RepoPath  Style `json:"repoPath"`
	GitOutput Style `json:"gitOutput"`

	// Branch
	BranchName      Style `json:"branchName"`
	BranchCurrent   Style `json:"branchCurrent"`
	BranchIndicator Style `json:"branchIndicator"`
	RemoteName      Style `json:"remoteName"`

	// Tag
	TagIndicator Style `json:"tagIndicator"`

	// Status
	Modified  Style `json:"modified"`
	Untracked Style `json:"untracked"`
	Diff      Style `json:"diff"`
	Error     Style `json:"error"`

	// Status table
	StatusHeader  Style `json:"statusHeader"`
	StatusFlag    Style `json:"statusFlag"`
	StatusAhead   Style `json:"statusAhead"`
	StatusBehind  Style `json:"statusBehind"`
	StatusAdded   Style `json:"statusAdded"`
	StatusDeleted Style `json:"statusDeleted"`
	StatusDim     Style `json:"statusDim"`
	StatusFooter  Style `json:"statusFooter"`

	// Table
	TableBorderStyle Style `json:"tableBorderStyle"`
	TableHeader      Style `json:"tableHeader"`
	TableRowEven     Style `json:"tableRowEven"`
	TableRowOdd      Style `json:"tableRowOdd"`
}
