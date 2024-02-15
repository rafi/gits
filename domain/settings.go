package domain

type Settings struct {
	Cache   *bool  `json:"cache,omitempty"`
	Finder  Finder `json:"finder"`
	Icons   Icons  `json:"icons"`
	Theme   Theme  `json:"theme"`
	Verbose bool   `json:"verbose,omitempty"`
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
	Normal        Style `json:"normal,omitempty"`
	Bullet        Style `json:"bullet,omitempty"`
	PreviewHeader Style `json:"previewHeader,omitempty"`

	// Project
	ProjectTitle Style `json:"projectTitle,omitempty"`
	Provider     Style `json:"provider,omitempty"`
	Desc         Style `json:"desc,omitempty"`

	// Repository
	RepoTitle Style `json:"repoTitle,omitempty"`
	RepoPath  Style `json:"repoPath,omitempty"`
	GitOutput Style `json:"gitOutput,omitempty"`

	// Branch
	BranchName      Style `json:"branchName,omitempty"`
	BranchCurrent   Style `json:"branchCurrent,omitempty"`
	BranchIndicator Style `json:"branchIndicator,omitempty"`
	RemoteName      Style `json:"remoteName,omitempty"`

	// Tag
	TagIndicator Style `json:"tagIndicator,omitempty"`

	// Status
	Modified  Style `json:"modified,omitempty"`
	Untracked Style `json:"untracked,omitempty"`
	Diff      Style `json:"diff,omitempty"`
	Error     Style `json:"error,omitempty"`

	// Table
	// TableBorder      Style `json:"tableBorder,omitempty"` FIXME:
	TableBorderStyle Style `json:"tableBorderStyle,omitempty"`
	TableHeader      Style `json:"tableHeader,omitempty"`
	TableRowEven     Style `json:"tableRowEven,omitempty"`
	TableRowOdd      Style `json:"tableRowOdd,omitempty"`
}
