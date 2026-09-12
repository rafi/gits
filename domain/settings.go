package domain

type Settings struct {
	Cache       *bool  `json:"cache,omitempty"`
	Finder      Finder `json:"finder"`
	Icons       Icons  `json:"icons"`
	Theme       Theme  `json:"theme"`
	Verbose     bool   `json:"verbose,omitempty"`
	WorkerCount int    `json:"workerCount,omitempty"`
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
