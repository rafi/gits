package list

import (
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/lipgloss/table"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/cli/types"
)

var (
	listHeaders         = []string{"TITLE", "STATE", "SOURCE"}
	listWideHeaders     = []string{"PATH"}
	listHeaderNamespace = "PROJECT"
)

// listWide lists projects in a wide table format.
func listWide(projects domain.ProjectListKeyed, deps types.RuntimeDeps) error {
	single := len(projects) == 1
	rows := [][]string{}
	headers := makeTableHeader(projects)
	headers = append(headers, listWideHeaders...)
	err := makeTableProjects(projects, single, true, deps.HomeDir, &rows)
	if err != nil {
		return fmt.Errorf("unable to draw wide table: %w", err)
	}

	return printTable(headers, rows, deps.Theme)
}

// listTable lists projects in a table format.
func listTable(projects domain.ProjectListKeyed, deps types.RuntimeDeps) error {
	single := len(projects) == 1
	rows := [][]string{}
	headers := makeTableHeader(projects)
	err := makeTableProjects(projects, single, false, deps.HomeDir, &rows)
	if err != nil {
		return fmt.Errorf("unable to draw table: %w", err)
	}

	return printTable(headers, rows, deps.Theme)
}

func printTable(headers []string, rows [][]string, theme types.Theme) error {
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

	fmt.Println(t)
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

// makeTableProjects recursively draws table rows.
func makeTableProjects(projects domain.ProjectListKeyed, single, wide bool, homeDir string, tableRows *[][]string) error {
	for _, proj := range projects {
		// Draw row columns, include project column if listing multiple projects.
		for _, repo := range proj.Repos {
			child := []string{}
			if !single {
				child = append(child, proj.Name)
			}
			child = append(child, repo.GetName(), string(repo.State), repo.GetSource())
			if wide {
				dir := cli.Path(repo.AbsPath, homeDir)
				child = append(child, dir)
			}
			*tableRows = append(*tableRows, child)
		}
		if len(proj.SubProjects) > 0 {
			subProjs := make(domain.ProjectListKeyed)
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
			err := makeTableProjects(subProjs, single, wide, homeDir, tableRows)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
