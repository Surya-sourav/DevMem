package docs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yourusername/devmem/internal/ai"
)

// WriteMasterDoc writes the human-readable master architecture markdown file.
func WriteMasterDoc(repoRoot string, analysis ai.MasterAnalysis, generatedAt time.Time) error {
	mermaid := buildMermaid(analysis)
	path := filepath.Join(repoRoot, ".devmem", "docs", "master-architecture.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create master docs directory: %w", err)
	}
	var b strings.Builder
	b.WriteString("# Architecture - " + analysis.ProjectName + "\n\n")
	b.WriteString("Generated at: " + generatedAt.UTC().Format(time.RFC3339) + "\n\n")
	b.WriteString("## Overview\n\n" + strings.TrimSpace(analysis.Overview) + "\n\n")
	b.WriteString("## Data flow\n\n" + strings.TrimSpace(analysis.DataFlow) + "\n\n")
	b.WriteString("## Tech stack\n\n")
	for _, item := range analysis.TechStack {
		b.WriteString("- " + item + "\n")
	}
	b.WriteString("\n## Modules\n\n")
	b.WriteString("| Name | Summary |\n| --- | --- |\n")
	for _, mod := range analysis.Modules {
		b.WriteString(fmt.Sprintf("| [%s](./modules/%s.md) | %s |\n", mod.Name, mod.Name, escapePipes(mod.OneLineSummary)))
	}
	b.WriteString("\n## Architecture diagram\n\n")
	b.WriteString("```mermaid\n")
	b.WriteString(mermaid)
	if !strings.HasSuffix(mermaid, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("```\n")
	return atomicWrite(path, []byte(b.String()))
}

// WriteMermaid writes the standalone mermaid graph file.
func WriteMermaid(repoRoot string, analysis ai.MasterAnalysis) error {
	path := filepath.Join(repoRoot, ".devmem", "docs", "master-architecture.mermaid")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create mermaid directory: %w", err)
	}
	return atomicWrite(path, []byte(buildMermaid(analysis)+"\n"))
}

func buildMermaid(analysis ai.MasterAnalysis) string {
	lines := []string{"graph TD"}
	nodes := map[string]string{}
	for _, mod := range analysis.Modules {
		nodes[sanitizeNodeID(mod.Name)] = mod.Name
	}
	for _, edge := range analysis.MermaidEdges {
		nodes[sanitizeNodeID(edge.From)] = edge.From
		nodes[sanitizeNodeID(edge.To)] = edge.To
	}
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		lines = append(lines, fmt.Sprintf("    %s[%s]", id, nodes[id]))
	}
	for _, edge := range analysis.MermaidEdges {
		from := sanitizeNodeID(edge.From)
		to := sanitizeNodeID(edge.To)
		label := strings.TrimSpace(edge.Label)
		if label != "" {
			lines = append(lines, fmt.Sprintf("    %s -->|%s| %s", from, label, to))
			continue
		}
		lines = append(lines, fmt.Sprintf("    %s --> %s", from, to))
	}
	return strings.Join(lines, "\n")
}

func sanitizeNodeID(name string) string {
	r := strings.NewReplacer(" ", "_", "-", "_", "/", "_")
	clean := r.Replace(strings.TrimSpace(name))
	if clean == "" {
		return "unknown"
	}
	return clean
}

func escapePipes(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}
