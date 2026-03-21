package crawler

import (
	"testing"

	"github.com/yourusername/devmem/internal/state"
)

func TestModuleScorerDetect(t *testing.T) {
	tree := &FileNode{
		Name: "repo",
		Path: ".",
		Type: "dir",
		Children: []*FileNode{
			{
				Name: "auth",
				Path: "auth",
				Type: "dir",
				Children: []*FileNode{
					{Name: "README.md", Path: "auth/README.md", Type: "file", Ext: ".md"},
					{Name: "main.go", Path: "auth/main.go", Type: "file", Ext: ".go"},
					{Name: "a.go", Path: "auth/a.go", Type: "file", Ext: ".go"},
					{Name: "b.go", Path: "auth/b.go", Type: "file", Ext: ".go"},
					{Name: "c.go", Path: "auth/c.go", Type: "file", Ext: ".go"},
					{Name: "d.go", Path: "auth/d.go", Type: "file", Ext: ".go"},
					{Name: "e.go", Path: "auth/e.go", Type: "file", Ext: ".go"},
				},
			},
			{
				Name: "pkg",
				Path: "pkg",
				Type: "dir",
				Children: []*FileNode{
					{Name: "util.go", Path: "pkg/util.go", Type: "file", Ext: ".go"},
				},
			},
		},
	}

	tests := []struct {
		name string
		cfg  *state.Config
		want int
	}{
		{name: "heuristic", cfg: nil, want: 1},
		{name: "config override", cfg: &state.Config{Modules: map[string]state.ModuleConfig{"core": {Paths: []string{"pkg"}}}}, want: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scorer := &ModuleScorer{Tree: tree, Config: tc.cfg}
			mods := scorer.Detect()
			if len(mods) != tc.want {
				t.Fatalf("expected %d modules, got %d", tc.want, len(mods))
			}
		})
	}
}
