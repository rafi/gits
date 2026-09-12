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

type Settings struct {
	Cache           *bool  `json:"cache,omitempty"`
	Finder          Finder `json:"finder"`
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
	DiffError string `json:"diffError,omitempty"`
	DiffClean string `json:"diffClean,omitempty"`
	Ahead     string `json:"ahead,omitempty"`
	Behind    string `json:"behind,omitempty"`
	NA        string `json:"na,omitempty"`
}

type Style struct {
	Color string `json:"color,omitempty"`
	Align string `json:"align,omitempty"`
	Width int    `json:"width,omitempty"`
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

	// Table
	TableBorderStyle Style `json:"tableBorderStyle"`
	TableHeader      Style `json:"tableHeader"`
	TableRowEven     Style `json:"tableRowEven"`
	TableRowOdd      Style `json:"tableRowOdd"`
}
