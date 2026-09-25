package theme

import (
	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/internal/app/cli/render/style"
)

// Role names one styled element. Its value is the key users write under
// settings.theme.styles.
type Role string

const (
	RoleNormal           Role = "normal"
	RoleBullet           Role = "bullet"
	RolePreviewHeader    Role = "previewHeader"
	RoleProjectTitle     Role = "projectTitle"
	RoleProvider         Role = "provider"
	RoleDesc             Role = "desc"
	RoleRepoTitle        Role = "repoTitle"
	RoleRepoPath         Role = "repoPath"
	RoleGitOutput        Role = "gitOutput"
	RoleBranchName       Role = "branchName"
	RoleBranchCurrent    Role = "branchCurrent"
	RoleBranchIndicator  Role = "branchIndicator"
	RoleRemoteName       Role = "remoteName"
	RoleTagIndicator     Role = "tagIndicator"
	RoleDiff             Role = "diff"
	RoleError            Role = "error"
	RoleWarning          Role = "warning"
	RoleStatusHeader     Role = "statusHeader"
	RoleStatusFlag       Role = "statusFlag"
	RoleStatusStaged     Role = "statusStaged"
	RoleStatusUnstaged   Role = "statusUnstaged"
	RoleStatusUntracked  Role = "statusUntracked"
	RoleStatusAhead      Role = "statusAhead"
	RoleStatusBehind     Role = "statusBehind"
	RoleStatusDiverged   Role = "statusDiverged"
	RoleStatusGone       Role = "statusGone"
	RoleStatusAdded      Role = "statusAdded"
	RoleStatusDeleted    Role = "statusDeleted"
	RoleStatusDim        Role = "statusDim"
	RoleStatusFooter     Role = "statusFooter"
	RoleStatusAgeFresh   Role = "statusAgeFresh"
	RoleStatusAgeRecent  Role = "statusAgeRecent"
	RoleStatusAgeStale   Role = "statusAgeStale"
	RoleTableBorderStyle Role = "tableBorderStyle"
	RoleTableHeader      Role = "tableHeader"
	RoleTableRowEven     Role = "tableRowEven"
	RoleTableRowOdd      Role = "tableRowOdd"
	RoleChartDates       Role = "chartDates"
	RoleProgressName     Role = "progressName"
	RoleProgressPercent  Role = "progressPercent"
	RoleProgressFill     Role = "progressFill"
	RoleProgressEmpty    Role = "progressEmpty"
	RoleProgressStats    Role = "progressStats"
	RoleProgressSpinner  Role = "progressSpinner"
)

// field returns a pointer to the style.Theme field a role styles.
type field func(*style.Theme) *lipgloss.Style

// roleFields binds each role to its style.Theme field. Explicit rather than
// reflected so a renamed field is a compile error, not a runtime panic.
var roleFields = map[Role]field{
	RoleNormal:           func(t *style.Theme) *lipgloss.Style { return &t.Normal },
	RoleBullet:           func(t *style.Theme) *lipgloss.Style { return &t.Bullet },
	RolePreviewHeader:    func(t *style.Theme) *lipgloss.Style { return &t.PreviewHeader },
	RoleProjectTitle:     func(t *style.Theme) *lipgloss.Style { return &t.ProjectTitle },
	RoleProvider:         func(t *style.Theme) *lipgloss.Style { return &t.Provider },
	RoleDesc:             func(t *style.Theme) *lipgloss.Style { return &t.Desc },
	RoleRepoTitle:        func(t *style.Theme) *lipgloss.Style { return &t.RepoTitle },
	RoleRepoPath:         func(t *style.Theme) *lipgloss.Style { return &t.RepoPath },
	RoleGitOutput:        func(t *style.Theme) *lipgloss.Style { return &t.GitOutput },
	RoleBranchName:       func(t *style.Theme) *lipgloss.Style { return &t.BranchName },
	RoleBranchCurrent:    func(t *style.Theme) *lipgloss.Style { return &t.BranchCurrent },
	RoleBranchIndicator:  func(t *style.Theme) *lipgloss.Style { return &t.BranchIndicator },
	RoleRemoteName:       func(t *style.Theme) *lipgloss.Style { return &t.RemoteName },
	RoleTagIndicator:     func(t *style.Theme) *lipgloss.Style { return &t.TagIndicator },
	RoleDiff:             func(t *style.Theme) *lipgloss.Style { return &t.Diff },
	RoleError:            func(t *style.Theme) *lipgloss.Style { return &t.Error },
	RoleWarning:          func(t *style.Theme) *lipgloss.Style { return &t.Warning },
	RoleStatusHeader:     func(t *style.Theme) *lipgloss.Style { return &t.StatusHeader },
	RoleStatusFlag:       func(t *style.Theme) *lipgloss.Style { return &t.StatusFlag },
	RoleStatusStaged:     func(t *style.Theme) *lipgloss.Style { return &t.StatusStaged },
	RoleStatusUnstaged:   func(t *style.Theme) *lipgloss.Style { return &t.StatusUnstaged },
	RoleStatusUntracked:  func(t *style.Theme) *lipgloss.Style { return &t.StatusUntracked },
	RoleStatusAhead:      func(t *style.Theme) *lipgloss.Style { return &t.StatusAhead },
	RoleStatusBehind:     func(t *style.Theme) *lipgloss.Style { return &t.StatusBehind },
	RoleStatusDiverged:   func(t *style.Theme) *lipgloss.Style { return &t.StatusDiverged },
	RoleStatusGone:       func(t *style.Theme) *lipgloss.Style { return &t.StatusGone },
	RoleStatusAdded:      func(t *style.Theme) *lipgloss.Style { return &t.StatusAdded },
	RoleStatusDeleted:    func(t *style.Theme) *lipgloss.Style { return &t.StatusDeleted },
	RoleStatusDim:        func(t *style.Theme) *lipgloss.Style { return &t.StatusDim },
	RoleStatusFooter:     func(t *style.Theme) *lipgloss.Style { return &t.StatusFooter },
	RoleStatusAgeFresh:   func(t *style.Theme) *lipgloss.Style { return &t.StatusAgeFresh },
	RoleStatusAgeRecent:  func(t *style.Theme) *lipgloss.Style { return &t.StatusAgeRecent },
	RoleStatusAgeStale:   func(t *style.Theme) *lipgloss.Style { return &t.StatusAgeStale },
	RoleTableBorderStyle: func(t *style.Theme) *lipgloss.Style { return &t.TableBorderStyle },
	RoleTableHeader:      func(t *style.Theme) *lipgloss.Style { return &t.TableHeader },
	RoleTableRowEven:     func(t *style.Theme) *lipgloss.Style { return &t.TableRowEven },
	RoleTableRowOdd:      func(t *style.Theme) *lipgloss.Style { return &t.TableRowOdd },
	RoleChartDates:       func(t *style.Theme) *lipgloss.Style { return &t.ChartDates },
	RoleProgressName:     func(t *style.Theme) *lipgloss.Style { return &t.ProgressName },
	RoleProgressPercent:  func(t *style.Theme) *lipgloss.Style { return &t.ProgressPercent },
	RoleProgressFill:     func(t *style.Theme) *lipgloss.Style { return &t.ProgressFill },
	RoleProgressEmpty:    func(t *style.Theme) *lipgloss.Style { return &t.ProgressEmpty },
	RoleProgressStats:    func(t *style.Theme) *lipgloss.Style { return &t.ProgressStats },
	RoleProgressSpinner:  func(t *style.Theme) *lipgloss.Style { return &t.ProgressSpinner },
}

// Roles lists every role in declaration order, for validation messages and
// documentation.
func Roles() []Role {
	return []Role{
		RoleNormal, RoleBullet, RolePreviewHeader, RoleProjectTitle, RoleProvider,
		RoleDesc, RoleRepoTitle, RoleRepoPath, RoleGitOutput, RoleBranchName,
		RoleBranchCurrent, RoleBranchIndicator, RoleRemoteName, RoleTagIndicator,
		RoleDiff, RoleError, RoleWarning, RoleStatusHeader, RoleStatusFlag,
		RoleStatusStaged, RoleStatusUnstaged, RoleStatusUntracked, RoleStatusAhead,
		RoleStatusBehind, RoleStatusDiverged, RoleStatusGone, RoleStatusAdded,
		RoleStatusDeleted, RoleStatusDim, RoleStatusFooter, RoleStatusAgeFresh,
		RoleStatusAgeRecent, RoleStatusAgeStale, RoleTableBorderStyle, RoleTableHeader,
		RoleTableRowEven, RoleTableRowOdd, RoleChartDates,
		RoleProgressName, RoleProgressPercent, RoleProgressFill, RoleProgressEmpty,
		RoleProgressStats, RoleProgressSpinner,
	}
}

// baseMapping is the one palette → role mapping every theme shares. A theme
// adjusts it through Tweaks; users through settings.theme.styles.
var baseMapping = map[Role]Spec{
	RoleNormal:           {},
	RoleBullet:           {Fg: Ref(SlotBlue)},
	RolePreviewHeader:    {},
	RoleProjectTitle:     {Fg: Ref(SlotMagenta), Bold: on},
	RoleProvider:         {Fg: Ref(SlotMuted)},
	RoleDesc:             {Fg: Ref(SlotFg)},
	RoleRepoTitle:        {Fg: Ref(SlotBlue), Bold: on},
	RoleRepoPath:         {Fg: Ref(SlotFg)},
	RoleGitOutput:        {Fg: Ref(SlotCyan)},
	RoleBranchName:       {Fg: Ref(SlotBlue)},
	RoleBranchCurrent:    {Fg: Ref(SlotBlue)},
	RoleBranchIndicator:  {Fg: Ref(SlotBlue), Bold: on},
	RoleRemoteName:       {Fg: Ref(SlotRed)},
	RoleTagIndicator:     {Fg: Ref(SlotRed), Bold: on},
	RoleDiff:             {Fg: Ref(SlotMagenta)},
	RoleError:            {Fg: Ref(SlotRed)},
	RoleWarning:          {Fg: Ref(SlotYellow), Faint: on},
	RoleStatusHeader:     {Bold: on},
	RoleStatusFlag:       {Fg: Ref(SlotCyan)},
	RoleStatusStaged:     {Fg: Ref(SlotGreen)},
	RoleStatusUnstaged:   {Fg: Ref(SlotYellow)},
	RoleStatusUntracked:  {Fg: Ref(SlotBlue)},
	RoleStatusAhead:      {Fg: Ref(SlotGreen)},
	RoleStatusBehind:     {Fg: Ref(SlotRed), Faint: on},
	RoleStatusDiverged:   {Fg: Ref(SlotYellow)},
	RoleStatusGone:       {Fg: Ref(SlotMagenta)},
	RoleStatusAdded:      {Fg: Ref(SlotGreen)},
	RoleStatusDeleted:    {Fg: Ref(SlotRed)},
	RoleStatusDim:        {Fg: Ref(SlotMuted)},
	RoleStatusFooter:     {Fg: Ref(SlotMuted)},
	RoleStatusAgeFresh:   {Fg: Ref(SlotGreen)},
	RoleStatusAgeRecent:  {Fg: Ref(SlotFg)},
	RoleStatusAgeStale:   {Fg: Ref(SlotMuted)},
	RoleTableBorderStyle: {Fg: Ref(SlotSubtle)},
	RoleTableHeader:      {Fg: Ref(SlotCyan)},
	RoleTableRowEven:     {Fg: Ref(SlotFg)},
	RoleTableRowOdd:      {Fg: Ref(SlotFg)},
	RoleChartDates:       {Fg: Ref(SlotMuted)},
	RoleProgressName:     {Fg: Ref(SlotBlue), Bold: on},
	RoleProgressPercent:  {Fg: Ref(SlotCyan)},
	RoleProgressFill:     {Fg: Ref(SlotGreen)},
	RoleProgressEmpty:    {Fg: Ref(SlotSubtle)},
	RoleProgressStats:    {Fg: Ref(SlotMuted)},
	RoleProgressSpinner:  {Fg: Ref(SlotAccent)},
}
