package add

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// load loads a YAML file into an abstract node. If no config exists, it
// initializes an empty mapping at the default path without writing it until
// the add operation succeeds.
func load(filePath, homeDir string) (yaml.Node, string, bool, error) {
	var node yaml.Node
	if filePath == "" {
		filePath = filepath.Join(homeDir, ".gits.yaml")
	}

	data, err := os.ReadFile(filePath)
	created := false
	initializedEmpty := false
	switch {
	case os.IsNotExist(err):
		data = []byte("{}\n")
		created = true
		initializedEmpty = true
	case err != nil:
		return node, filePath, false, err
	case len(strings.TrimSpace(string(data))) == 0:
		data = []byte("{}\n")
		initializedEmpty = true
	}

	if err := yaml.Unmarshal(data, &node); err != nil {
		return node, filePath, false, err
	}
	if len(node.Content) == 0 || node.Content[0].Kind != yaml.MappingNode {
		return node, filePath, false, fmt.Errorf("config root must be a YAML mapping")
	}
	if initializedEmpty {
		node.Content[0].Style = 0
	}
	return node, filePath, created, nil
}

// save saves a yaml node into a file.
func save(filePath string, node yaml.Node) error {
	configDir := filepath.Dir(filePath)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(configDir, ".gits-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer tmpFile.Close()
	defer os.Remove(tmpPath)

	enc := yaml.NewEncoder(tmpFile)
	enc.SetIndent(2)
	if err := enc.Encode(&node); err != nil {
		return err
	}

	if err := enc.Close(); err != nil {
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, filePath)
}

// appendProject appends a project node to the root node.
func appendProject(projectName string, node *yaml.Node) {
	node.Content = append(node.Content, &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: projectName,
	})
	node.Content = append(node.Content, &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "repos"},
			{Kind: yaml.SequenceNode, Content: []*yaml.Node{}},
		},
	})
}

// appendRepo appends a repository to a project node.
func appendRepo(path, remoteSrc string, node *yaml.Node) {
	node.Content = append(node.Content, &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "dir"},
			{Kind: yaml.ScalarNode, Value: path},
			{Kind: yaml.ScalarNode, Value: "src"},
			{Kind: yaml.ScalarNode, Value: remoteSrc},
		},
	})
}

// findProject finds a project node in the config file.
func findProject(projectName string, rootNode *yaml.Node) (*yaml.Node, error) {
	for i := 0; i < len(rootNode.Content[0].Content); i++ {
		node := rootNode.Content[0].Content[i]
		if node.Kind == yaml.ScalarNode && node.Value == projectName {
			return rootNode.Content[0].Content[i+1], nil
		}
	}
	return nil, fmt.Errorf("unable to find project %q in config", projectName)
}

// findScalarMapping finds a scalar mapping in a node.
func findScalarMapping(nodeName string, nodes *yaml.Node) (*yaml.Node, error) {
	for i := 0; i < len(nodes.Content); i++ {
		node := nodes.Content[i]
		if node.Kind == yaml.ScalarNode && node.Value == nodeName {
			return nodes.Content[i+1], nil
		}
	}
	return nil, fmt.Errorf("unable to find node %q in config", nodeName)
}
