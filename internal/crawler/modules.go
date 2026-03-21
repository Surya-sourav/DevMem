package crawler

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/Surya-sourav/devmem/internal/state"
)

// Module describes a detected documentation module.
type Module struct {
	Name      string   `json:"name"`
	RootPath  string   `json:"root_path"`
	Paths     []string `json:"paths"`
	EntryFile string   `json:"entry_file"`
	FileCount int      `json:"file_count"`
	Score     int      `json:"score"`
	Source    string   `json:"source"`
}

// ModuleScorer detects module boundaries from the tree.
type ModuleScorer struct {
	Tree   *FileNode
	Config *state.Config
}

var entryCandidates = map[string]struct{}{
	"main.go":     {},
	"index.ts":    {},
	"index.js":    {},
	"index.html":  {},
	"main.ts":     {},
	"main.js":     {},
	"app.py":      {},
	"__init__.py": {},
	"mod.rs":      {},
}

var containerDirs = map[string]struct{}{
	"internal": {},
	"pkg":      {},
	"vendor":   {},
}

// Detect discovers modules from explicit config or heuristic analysis.
func (m *ModuleScorer) Detect() []Module {
	if m.Tree == nil {
		return nil
	}
	mods := make([]Module, 0)
	if m.Config != nil && len(m.Config.Modules) > 0 {
		for name, cfg := range m.Config.Modules {
			paths := append([]string(nil), cfg.Paths...)
			if len(paths) == 0 {
				continue
			}
			mods = append(mods, Module{
				Name:     name,
				RootPath: filepath.ToSlash(paths[0]),
				Paths:    paths,
				Score:    99,
				Source:   "config",
			})
		}
		sort.Slice(mods, func(i, j int) bool { return mods[i].Name < mods[j].Name })
		return mods
	}

	mods = m.detectNode(m.Tree, 0)
	mods = deduplicate(mods)
	sort.Slice(mods, func(i, j int) bool {
		if mods[i].Score != mods[j].Score {
			return mods[i].Score > mods[j].Score
		}
		return mods[i].Name < mods[j].Name
	})
	return mods
}

func (m *ModuleScorer) detectNode(node *FileNode, depth int) []Module {
	if node == nil || node.Type != "dir" {
		return nil
	}
	if node.Path != "." {
		if _, isContainer := containerDirs[strings.ToLower(node.Name)]; isContainer {
			out := make([]Module, 0)
			for _, child := range node.Children {
				if child.Type != "dir" {
					continue
				}
				out = append(out, m.detectNode(child, depth+1)...)
			}
			return out
		}

		score, entry, count := scoreDirectory(node, depth)
		if score >= 3 {
			name := filepath.Base(node.Path)
			return []Module{{
				Name:      sanitizeName(name),
				RootPath:  node.Path,
				Paths:     []string{node.Path},
				EntryFile: entry,
				FileCount: count,
				Score:     score,
				Source:    "heuristic",
			}}
		}
	}

	out := make([]Module, 0)
	for _, child := range node.Children {
		if child.Type != "dir" {
			continue
		}
		out = append(out, m.detectNode(child, depth+1)...)
	}
	return out
}

func scoreDirectory(node *FileNode, depth int) (score int, entry string, fileCount int) {
	for _, child := range node.Children {
		if child.Type == "file" {
			if strings.EqualFold(child.Name, "README.md") {
				score += 3
			}
			if _, ok := entryCandidates[child.Name]; ok {
				score += 2
				entry = child.Path
			}
		}
	}
	if depth >= 1 && depth <= 3 {
		score += 2
	}
	fileCount = sourceFileCount(node)
	if fileCount >= 2 {
		score++
	}
	return score, entry, fileCount
}

func sanitizeName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.ReplaceAll(name, " ", "-")
	return strings.ReplaceAll(name, "_", "-")
}

func deduplicate(modules []Module) []Module {
	out := make([]Module, 0, len(modules))
	for _, candidate := range modules {
		covered := false
		for _, existing := range out {
			if existing.Source == "config" && pathCovered(candidate.RootPath, existing.Paths) {
				covered = true
				break
			}
			if pathCovered(candidate.RootPath, existing.Paths) && existing.Score >= candidate.Score {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, candidate)
		}
	}
	return out
}

func pathCovered(path string, roots []string) bool {
	path = filepath.ToSlash(path)
	for _, root := range roots {
		root = filepath.ToSlash(root)
		if path == root || strings.HasPrefix(path, root+"/") {
			return true
		}
	}
	return false
}

func sourceFileCount(node *FileNode) int {
	exts := map[string]struct{}{
		".go": {}, ".ts": {}, ".js": {}, ".py": {}, ".rs": {}, ".rb": {}, ".java": {}, ".cs": {}, ".cpp": {}, ".c": {}, ".swift": {}, ".kt": {},
		".tsx": {}, ".jsx": {}, ".html": {}, ".css": {}, ".scss": {}, ".sass": {}, ".vue": {}, ".svelte": {},
	}
	count := 0
	var walk func(*FileNode)
	walk = func(n *FileNode) {
		if n == nil {
			return
		}
		if n.Type == "file" {
			if _, ok := exts[strings.ToLower(n.Ext)]; ok {
				count++
			}
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(node)
	return count
}
