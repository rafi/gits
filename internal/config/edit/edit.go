// Package edit changes a gits config file in place, as a YAML syntax tree, so
// that writing it back alters only the lines an edit adds: comments, blank
// lines, quoting, and the indentation the user chose all survive as they were
// written.
//
// It is the one writer of the config file. `gits add` appends a Repository to
// a Project's `repos:`; `gits discover` appends whole Projects with a Project
// Path. Both go through here, so a config file is rewritten by one set of
// rules rather than by each command's own.
package edit

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"

	"github.com/rafi/gits/internal/infra/fsutil"
)

// Doc is a config file held as a YAML syntax tree, so that writing it
// back changes only the lines `gits add` appends: comments, blank lines,
// quoting, and indentation the user chose all survive as they were.
type Doc struct {
	file *ast.File
	// original is the file as read, and pristine is the syntax tree printed
	// back before any change. Where the two differ only in whitespace, save
	// keeps the original's spelling of lines the edit did not touch.
	original []byte
	pristine string
	// root is the top-level mapping of project names. It is nil for a file
	// with nothing in it but whitespace or comments, and is created by the
	// first addProject.
	root *ast.MappingNode
}

// Load parses a yaml file into a Doc. A file that is empty, or absent
// at a path the caller chose, is one to append to rather than a failure — the
// first `gits add` is exactly how a config gets started.
func Load(filePath string) (*Doc, error) {
	if filePath == "" {
		return nil, errors.New("no config file found: pass one with `-c`")
	}
	data, err := os.ReadFile(filePath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	file, err := parser.ParseBytes(data, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	switch len(file.Docs) {
	case 0:
		file.Docs = append(file.Docs, &ast.DocumentNode{})
	case 1:
	default:
		return nil, errNotAMapping
	}
	cf := &Doc{file: file, original: data, pristine: file.String()}
	switch body := file.Docs[0].Body.(type) {
	case nil, *ast.CommentGroupNode:
		// Nothing but whitespace or comments: the first AddProject makes the
		// root mapping, and keeps the comments above it.
	case *ast.MappingNode:
		cf.root = body
	default:
		return nil, errNotAMapping
	}
	return cf, nil
}

// errNotAMapping is what load reports for a config file whose top level is not
// a mapping of project names, which nothing here could append to.
var errNotAMapping = errors.New("config file is not a mapping of projects")

// yamlIndent is the indentation new blocks are written with, when the file has
// no block of its own to copy the style from.
const yamlIndent = 2

// Save writes the config file back, atomically: the original file mode is
// preserved, and any failure leaves no temp file behind.
func (cf *Doc) Save(filePath string) error {
	data := []byte(splice(string(cf.original), cf.pristine, cf.file.String()))
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	return fsutil.WriteFileAtomicPreserve(filePath, data, 0o644)
}

// splice rewrites edited, the printed syntax tree after an edit, so that every
// line the edit left alone reads exactly as it did in original. The printer
// normalizes whitespace it does not track, such as the run of spaces before an
// inline comment, so lines common to pristine (the tree printed before the
// edit) and edited are looked up in original by line number. That holds when
// original and pristine have the same number of lines and each pair differs
// only in whitespace; otherwise edited is returned as printed.
func splice(original, pristine, edited string) string {
	origLines := strings.Split(strings.TrimSuffix(original, "\n"), "\n")
	oldLines := strings.Split(strings.TrimSuffix(pristine, "\n"), "\n")
	newLines := strings.Split(strings.TrimSuffix(edited, "\n"), "\n")
	if len(origLines) != len(oldLines) {
		return edited
	}
	for i := range oldLines {
		if !sameIgnoringSpace(origLines[i], oldLines[i]) {
			return edited
		}
	}

	// Longest common subsequence of lines between the tree before and after
	// the edit, so a line is only taken from original when it is truly the
	// same line and not one the edit rewrote.
	lcs := make([][]int, len(oldLines)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(newLines)+1)
	}
	for i := range slices.Backward(oldLines) {
		for j := range slices.Backward(newLines) {
			if oldLines[i] == newLines[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	out := make([]string, 0, len(newLines))
	i, j := 0, 0
	for i < len(oldLines) && j < len(newLines) {
		switch {
		case oldLines[i] == newLines[j]:
			out = append(out, origLines[i])
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			i++ // a line the edit removed
		default:
			out = append(out, newLines[j]) // a line the edit added
			j++
		}
	}
	out = append(out, newLines[j:]...)
	return strings.Join(out, "\n") + "\n"
}

// sameIgnoringSpace reports whether two lines differ in nothing but spaces.
func sameIgnoringSpace(a, b string) bool {
	return strings.ReplaceAll(a, " ", "") == strings.ReplaceAll(b, " ", "")
}

// repoEntry is one item under a project's `repos:`, in the key order the
// config file documents.
type repoEntry struct {
	Dir string `yaml:"dir"`
	Src string `yaml:"src"`
}

// FindProject finds a project's mapping by name.
func (cf *Doc) FindProject(projectName string) (*ast.MappingNode, error) {
	if cf.root == nil {
		return nil, fmt.Errorf("unable to find project %q in config", projectName)
	}
	value := findKey(cf.root, projectName)
	if value == nil {
		return nil, fmt.Errorf("unable to find project %q in config", projectName)
	}
	project, ok := value.Value.(*ast.MappingNode)
	if !ok {
		return nil, fmt.Errorf("project %q in config is not a mapping", projectName)
	}
	return project, nil
}

// AddProject appends a project with an empty `repos:` to the root mapping,
// and returns its mapping node.
func (cf *Doc) AddProject(projectName string) (*ast.MappingNode, error) {
	return cf.addProject(projectName, map[string]any{"repos": nil})
}

// AddProjectPath appends a project that is nothing but a Project Path, which
// is the whole of a project discovered from the filesystem: the loader gives
// a project with a path and no `repos:` a filesystem source of its own, so
// the entry stays one line per project however many repositories come and go
// under it. A description is written when one is given and omitted when not.
func (cf *Doc) AddProjectPath(projectName, path, desc string) error {
	body := map[string]any{"path": path}
	if desc != "" {
		body["desc"] = desc
	}
	_, err := cf.addProject(projectName, body)
	return err
}

// HasProject reports whether the config file already holds a project under
// this name. It asks the file rather than the loaded project list, so a name
// taken by an entry the loader skipped still counts as taken.
func (cf *Doc) HasProject(projectName string) bool {
	if cf.root == nil {
		return false
	}
	return findKey(cf.root, projectName) != nil
}

// addProject appends a project with the given body to the root mapping, and
// returns its mapping node.
func (cf *Doc) addProject(projectName string, body map[string]any) (*ast.MappingNode, error) {
	frag, err := mappingFragment(map[string]any{projectName: body})
	if err != nil {
		return nil, err
	}
	if cf.root == nil {
		// The root mapping is new. When the file was comments only, they
		// stay above it.
		if comments, ok := cf.file.Docs[0].Body.(*ast.CommentGroupNode); ok {
			if err := frag.SetComment(comments); err != nil {
				return nil, err
			}
		}
		cf.file.Docs[0].Body = frag
		cf.root = frag
	} else if err := ast.Merge(cf.root, frag); err != nil {
		return nil, err
	}
	return cf.FindProject(projectName)
}

// AddRepo appends a repository to a project's `repos:`. The new item takes
// the style of the list it joins: a flow list `[...]` gets another flow item;
// a block list gets another `- dir:` at its own indentation; a `repos:` that
// is missing, null, or `[]` becomes a block list indented like the rest of
// the file.
func (cf *Doc) AddRepo(project *ast.MappingNode, dir, src string) error {
	entry := repoEntry{Dir: dir, Src: src}
	repos := findKey(project, "repos")
	if repos == nil {
		frag, err := mappingFragment(map[string]any{"repos": []repoEntry{entry}})
		if err != nil {
			return err
		}
		return ast.Merge(project, frag)
	}

	switch list := repos.Value.(type) {
	case *ast.SequenceNode:
		if len(list.Values) > 0 {
			frag, err := fragment([]repoEntry{entry}, yaml.Flow(list.IsFlowStyle))
			if err != nil {
				return err
			}
			return ast.Merge(list, frag)
		}
	case *ast.NullNode:
	default:
		return fmt.Errorf("`repos` of project %q in config is not a list",
			keyName(project))
	}

	// `repos: []` or `repos:` with nothing: start a block list. A comment
	// that sat after the old value moves to the key, so it is not lost.
	frag, err := fragment([]repoEntry{entry})
	if err != nil {
		return err
	}
	// Indent the list one level past its key, as the file would have had it.
	wantColumn := repos.Key.GetToken().Position.Column + yamlIndent
	frag.AddColumn(wantColumn - frag.GetToken().Position.Column)
	if comment := repos.Value.GetComment(); comment != nil && repos.Key.GetComment() == nil {
		if err := repos.Key.SetComment(comment); err != nil {
			return err
		}
	}
	repos.Value = frag
	return nil
}

// findKey finds a key's entry in a mapping, by the name the key *means*
// rather than the way it is spelled in the file.
//
// A project name that YAML would otherwise read as something other than a
// string — `123`, `yes`, `no`, `null`, `1.5` — is written quoted, and
// [ast.Node.String] gives back the quoted source, `"123"`. Comparing that to
// the name the caller asked for would miss every such project: the key would
// be written and then not found again, which is exactly what a directory
// named `123` did. Scalar values are unquoted before they are compared, so
// `"123"` in the file matches the name `123`.
func findKey(mapping *ast.MappingNode, key string) *ast.MappingValueNode {
	for _, value := range mapping.Values {
		if keyString(value.Key) == key {
			return value
		}
	}
	return nil
}

// keyString is the string a mapping key denotes, with any quoting the file
// spells it with removed. A non-scalar key — which no config file gits reads
// would have — falls back to its source form, so it simply fails to match.
func keyString(key ast.MapKeyNode) string {
	if scalar, ok := key.(ast.ScalarNode); ok {
		if str, ok := scalar.GetValue().(string); ok {
			return str
		}
	}
	return key.String()
}

// keyName is the name a project mapping sits under, for error messages.
func keyName(mapping *ast.MappingNode) string {
	if path := mapping.GetPath(); path != "" {
		return path
	}
	return "?"
}

// fragment marshals a value and parses it back as a syntax tree, so the new
// nodes are quoted and laid out by the same rules as a whole document, then
// re-indented by whoever grafts them onto the config file.
func fragment(value any, opts ...yaml.EncodeOption) (ast.Node, error) {
	opts = append([]yaml.EncodeOption{yaml.Indent(yamlIndent), yaml.IndentSequence(true)}, opts...)
	data, err := yaml.MarshalWithOptions(value, opts...)
	if err != nil {
		return nil, err
	}
	file, err := parser.ParseBytes(data, 0)
	if err != nil {
		return nil, err
	}
	return file.Docs[0].Body, nil
}

// mappingFragment is fragment for a value that marshals to a mapping.
func mappingFragment(value any) (*ast.MappingNode, error) {
	node, err := fragment(value)
	if err != nil {
		return nil, err
	}
	mapping, ok := node.(*ast.MappingNode)
	if !ok {
		return nil, fmt.Errorf("yaml fragment is %T, want a mapping", node)
	}
	return mapping, nil
}
