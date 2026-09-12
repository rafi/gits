package add

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/rafi/gits/internal/fsutil"
)

// load loads a yaml file into an abstract node.
func load(filePath string) (yaml.Node, error) {
	var node yaml.Node
	data, err := os.ReadFile(filePath)
	if err != nil {
		return node, err
	}
	err = yaml.Unmarshal(data, &node)
	return node, err
}

// save saves a yaml node into a file, atomically: the original file mode is
// preserved, and any failure leaves no temp file behind.
func save(filePath string, node yaml.Node) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&node); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return fsutil.WriteFileAtomicPreserve(filePath, buf.Bytes(), 0o644)
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
