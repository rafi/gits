package domain

type Settings struct {
	Cache   *bool `json:"cache,omitempty"`
	Theme   Theme `json:"theme"`
	Verbose bool  `json:"verbose,omitempty"`
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
