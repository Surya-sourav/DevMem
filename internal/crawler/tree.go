package crawler

// CountFiles returns the total file count in a FileNode tree.
func CountFiles(node *FileNode) int {
	if node == nil {
		return 0
	}
	if node.Type == "file" {
		return 1
	}
	total := 0
	for _, child := range node.Children {
		total += CountFiles(child)
	}
	return total
}

// FindNode locates a node by relative path.
func FindNode(node *FileNode, relPath string) *FileNode {
	if node == nil {
		return nil
	}
	if node.Path == relPath {
		return node
	}
	for _, child := range node.Children {
		if found := FindNode(child, relPath); found != nil {
			return found
		}
	}
	return nil
}
