package list

import (
	"path/filepath"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
)

var (
	listHeaders         = []string{"TITLE", "STATE", "SOURCE"}
	listWideHeaders     = []string{"PATH"}
	listHeaderNamespace = "PROJECT"
)

// listWide lists projects in a wide table format.
func listWide(projects domain.ProjectListKeyed, deps types.RuntimeCLI) error {
	single := len(projects) == 1
	headers := makeTableHeader(projects)
	headers = append(headers, listWideHeaders...)
	rows := makeTableProjects(projects, single, true, deps.HomeDir)

	return printTable(headers, rows, deps)
}

// listTable lists projects in a table format.
func listTable(projects domain.ProjectListKeyed, deps types.RuntimeCLI) error {
	single := len(projects) == 1
	headers := makeTableHeader(projects)
	rows := makeTableProjects(projects, single, false, deps.HomeDir)

	return printTable(headers, rows, deps)
}

func printTable(headers []string, rows [][]string, deps types.RuntimeCLI) error {
	theme := deps.Theme
	t := table.New().
		Border(theme.TableBorder).
		BorderStyle(theme.TableBorderStyle).
		BorderTop(false).
		BorderRight(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderColumn(true).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(theme.TableRowStyle)

	lipgloss.Fprintln(deps.Out, t)
	return nil
}

func makeTableHeader(projects domain.ProjectListKeyed) (header []string) {
	// Include project column if multiple projects included in arguments.
	if len(projects) > 1 {
		header = append(header, listHeaderNamespace)
	}
	header = append(header, listHeaders...)
	return header
}

// makeTableProjects recursively builds table rows. Projects are walked in
// SortedNames order so two runs render the same table; within a project the
// loader already sorted repositories and sub-projects (see sortTree).
func makeTableProjects(projects domain.ProjectListKeyed, single, wide bool, homeDir string) [][]string {
	rows := [][]string{}
	for _, name := range projects.SortedNames() {
		proj := projects[name]
		// Draw row columns, include project column if listing multiple projects.
		for _, repo := range proj.Repos {
			rows = append(rows, makeTableRow(proj, repo, single, wide, homeDir))
		}
		if len(proj.SubProjects) > 0 {
			subProjs := qualifySubProjects(proj, single)
			rows = append(rows, makeTableProjects(subProjs, single, wide, homeDir)...)
		}
	}
	return rows
}

// makeTableRow renders one repository's cells, in header order.
func makeTableRow(proj domain.Project, repo domain.Repository, single, wide bool, homeDir string) []string {
	row := []string{}
	if !single {
		row = append(row, proj.Name)
	}
	row = append(row, repo.GetName(), string(repo.State), repo.GetSource())
	if wide {
		row = append(row, cli.Path(repo.AbsPath, homeDir))
	}
	return row
}

// qualifySubProjects keys a project's sub-projects for the recursive call,
// prefixing the parent's name onto whichever column carries it: the
// repository name when a single project is listed and there is no project
// column, the sub-project's own name otherwise.
func qualifySubProjects(proj domain.Project, single bool) domain.ProjectListKeyed {
	subProjs := make(domain.ProjectListKeyed, len(proj.SubProjects))
	for _, subProj := range proj.SubProjects {
		if single {
			for idx, repo := range subProj.Repos {
				subProj.Repos[idx].Name = filepath.Join(subProj.Name, repo.Name)
			}
		} else {
			subProj.Name = filepath.Join(proj.Name, subProj.Name)
		}
		subProjs[subProj.Name] = subProj
	}
	return subProjs
}
