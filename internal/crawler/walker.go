package crawler

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileNode represents one node in the repository tree.
type FileNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	Type     string      `json:"type"`
	Ext      string      `json:"ext,omitempty"`
	Size     int64       `json:"size,omitempty"`
	Children []*FileNode `json:"children,omitempty"`
}

// Walker traverses a repository tree.
type Walker struct {
	Root        string
	IgnoreRules []string
}

// Walk recursively crawls the filesystem and returns a FileNode tree.
func (w *Walker) Walk() (*FileNode, error) {
	rootInfo, err := os.Lstat(w.Root)
	if err != nil {
		return nil, fmt.Errorf("stat root: %w", err)
	}
	if !rootInfo.IsDir() {
		return nil, fmt.Errorf("root is not a directory: %s", w.Root)
	}
	return w.walkDir(w.Root, ".")
}

func (w *Walker) walkDir(absPath, relPath string) (*FileNode, error) {
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, fmt.Errorf("read directory %s: %w", absPath, err)
	}
	node := &FileNode{
		Name: filepath.Base(absPath),
		Path: filepath.ToSlash(relPath),
		Type: "dir",
	}
	if relPath == "." {
		node.Name = filepath.Base(w.Root)
		node.Path = "."
	}

	for _, entry := range entries {
		name := entry.Name()
		childRel := filepath.ToSlash(filepath.Join(relPath, name))
		if relPath == "." {
			childRel = filepath.ToSlash(name)
		}
		if shouldIgnore(name, childRel, w.IgnoreRules) {
			continue
		}
		childAbs := filepath.Join(absPath, name)
		info, err := os.Lstat(childAbs)
		if err != nil {
			return nil, fmt.Errorf("stat path %s: %w", childAbs, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if info.IsDir() {
			child, err := w.walkDir(childAbs, childRel)
			if err != nil {
				return nil, err
			}
			node.Children = append(node.Children, child)
			continue
		}
		node.Children = append(node.Children, &FileNode{
			Name: name,
			Path: childRel,
			Type: "file",
			Ext:  strings.ToLower(filepath.Ext(name)),
			Size: info.Size(),
		})
	}
	sort.Slice(node.Children, func(i, j int) bool {
		if node.Children[i].Type != node.Children[j].Type {
			return node.Children[i].Type == "dir"
		}
		return node.Children[i].Name < node.Children[j].Name
	})
	return node, nil
}

func shouldIgnore(name, relPath string, rules []string) bool {
	for _, rule := range rules {
		r := strings.TrimSpace(rule)
		if r == "" || strings.HasPrefix(r, "#") {
			continue
		}
		r = strings.TrimPrefix(filepath.ToSlash(r), "./")
		candidate := filepath.ToSlash(relPath)
		base := filepath.Base(candidate)
		if base == r || candidate == r || strings.HasPrefix(candidate, r+"/") {
			return true
		}
		if matched, _ := filepath.Match(r, base); matched {
			return true
		}
		if matched, _ := filepath.Match(r, candidate); matched {
			return true
		}
	}
	return false
}

// TreeToString renders the tree into a compact human-readable string.
func TreeToString(node *FileNode, indent int) string {
	if node == nil {
		return ""
	}
	var b strings.Builder
	prefix := strings.Repeat("  ", indent)
	if node.Type == "dir" {
		if node.Path != "." {
			b.WriteString(fmt.Sprintf("%s%s/\n", prefix, node.Name))
		}
		for _, child := range node.Children {
			childIndent := indent
			if node.Path != "." {
				childIndent++
			}
			b.WriteString(TreeToString(child, childIndent))
		}
		return b.String()
	}
	b.WriteString(fmt.Sprintf("%s%-20s %s\n", prefix, node.Name, humanSize(node.Size)))
	return b.String()
}

// LoadIgnoreFile returns cleaned ignore patterns from the provided file.
func LoadIgnoreFile(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(b), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

func humanSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	v := float64(size)
	idx := 0
	for v >= 1024 && idx < len(units)-1 {
		v /= 1024
		idx++
	}
	if idx == 0 {
		return fmt.Sprintf("%.1f %s", float64(size)/1024.0, units[idx])
	}
	return fmt.Sprintf("%.1f %s", v, units[idx])
}
