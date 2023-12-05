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
	// Status
	Modified  Style `json:"modified,omitempty"`
	Untracked Style `json:"untracked,omitempty"`
	Diff      Style `json:"diff,omitempty"`
	Error     Style `json:"error,omitempty"`

	// Project
	ProjectTitle Style `json:"projectTitle,omitempty"`
	Bullet       Style `json:"bullet,omitempty"`
	Provider     Style `json:"provider,omitempty"`
	Desc         Style `json:"desc,omitempty"`

	// Repository
	RepoTitle Style `json:"repoTitle,omitempty"`
	GitOutput Style `json:"gitOutput,omitempty"`

	// Table
	// TableBorder      Style `json:"tableBorder,omitempty"`
	TableBorderStyle Style `json:"tableBorderStyle,omitempty"`
	TableHeader      Style `json:"tableHeader,omitempty"`
	TableRowEven     Style `json:"tableRowEven,omitempty"`
	TableRowOdd      Style `json:"tableRowOdd,omitempty"`
}
