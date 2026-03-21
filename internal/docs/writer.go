package docs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/Surya-sourav/devmem/internal/ai"
	"github.com/Surya-sourav/devmem/internal/crawler"
)

var moduleDocTemplate = template.Must(template.New("module-doc").Parse(`---
module: {{.Module}}
root_path: {{.RootPath}}
key_files:
{{- if .KeyFiles}}
{{- range .KeyFiles}}
  - {{.}}
{{- end}}
{{- else}}
  []
{{- end}}
depends_on:
{{- if .DependsOn}}
{{- range .DependsOn}}
  - {{.}}
{{- end}}
{{- else}}
  []
{{- end}}
changed_in: []
generated_at: {{.GeneratedAt}}
---

# {{.Module}}

{{.Description}}

## Purpose

{{.Purpose}}

## Key files
{{- if .KeyFiles}}
{{- range .KeyFiles}}

- {{.}}
{{- end}}
{{- else}}

- No key files identified.
{{- end}}

## Public API
{{- if .PublicAPI}}
{{- range .PublicAPI}}

- {{.}}
{{- end}}
{{- else}}

- No public API identified.
{{- end}}

## Dependencies
{{- if .DependsOn}}
{{- range .DependsOn}}

- {{.}}
{{- end}}
{{- else}}

- No module dependencies identified.
{{- end}}

## Tech notes

{{.TechNotes}}

## Changelog

No changes captured yet. Run devmem capture after your next commit.
`))

// WriteModuleDoc renders and writes one module document atomically.
func WriteModuleDoc(repoRoot string, mod crawler.Module, analysis ai.ModuleAnalysis) error {
	outPath := filepath.Join(repoRoot, ".devmem", "docs", "modules", mod.Name+".md")
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create module docs directory: %w", err)
	}
	keyFiles := append([]string(nil), analysis.KeyFiles...)
	depends := append([]string(nil), analysis.DependsOn...)
	sort.Strings(keyFiles)
	sort.Strings(depends)
	payload := struct {
		Module      string
		RootPath    string
		KeyFiles    []string
		DependsOn   []string
		GeneratedAt string
		Description string
		Purpose     string
		PublicAPI   []string
		TechNotes   string
	}{
		Module:      mod.Name,
		RootPath:    mod.RootPath,
		KeyFiles:    keyFiles,
		DependsOn:   depends,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Description: analysis.Description,
		Purpose:     analysis.Purpose,
		PublicAPI:   analysis.PublicAPI,
		TechNotes:   analysis.TechNotes,
	}
	var b strings.Builder
	if err := moduleDocTemplate.Execute(&b, payload); err != nil {
		return fmt.Errorf("render module doc: %w", err)
	}
	return atomicWrite(outPath, []byte(b.String()))
}

// PatchModuleDoc updates named sections and appends a compact changelog entry.
func PatchModuleDoc(repoRoot string, moduleName string, patch ai.DocPatch, commitHash string, shortSummary string) error {
	path := filepath.Join(repoRoot, ".devmem", "docs", "modules", moduleName+".md")
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read module doc: %w", err)
	}
	fm, body, err := ParseFrontmatter(string(content))
	if err != nil {
		return fmt.Errorf("parse module doc frontmatter: %w", err)
	}
	for heading, replacement := range patch.Sections {
		if strings.EqualFold(strings.TrimSpace(heading), "changelog") {
			continue
		}
		body = replaceSection(body, heading, replacement)
	}
	changedIn := toStringSlice(fm["changed_in"])
	if !contains(changedIn, commitHash) {
		changedIn = append(changedIn, commitHash)
	}
	body = ensureChangelogSection(body, changedIn, commitHash, shortSummary)
	fm["changed_in"] = changedIn
	updated, err := RenderFrontmatter(fm, body)
	if err != nil {
		return fmt.Errorf("render patched doc: %w", err)
	}
	return atomicWrite(path, []byte(updated))
}

// WriteChangelogEntry writes one changelog markdown entry atomically.
func WriteChangelogEntry(repoRoot string, commitHash string, classification ai.ChangeClassification, date time.Time) error {
	path := filepath.Join(repoRoot, ".devmem", "changelog", commitHash+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create changelog directory: %w", err)
	}
	mods := make([]string, 0, len(classification.Modules))
	for name := range classification.Modules {
		mods = append(mods, name)
	}
	sort.Strings(mods)
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("id: " + commitHash + "\n")
	b.WriteString("date: " + date.UTC().Format(time.RFC3339) + "\n")
	b.WriteString("type: " + classification.Type + "\n")
	b.WriteString(fmt.Sprintf("breaking: %t\n", classification.Breaking))
	b.WriteString("modules:\n")
	for _, m := range mods {
		b.WriteString("  - " + m + "\n")
	}
	b.WriteString("---\n\n")
	summary := strings.TrimSpace(classification.Summary)
	if summary == "" {
		summary = "No summary provided."
	}
	b.WriteString(summary + "\n")
	if len(mods) > 0 {
		b.WriteString("\nModule updates:\n")
		for _, m := range mods {
			desc := strings.TrimSpace(classification.Modules[m])
			if desc == "" {
				desc = "No module-specific summary provided."
			}
			b.WriteString("- " + m + ": " + desc + "\n")
		}
	}
	return atomicWrite(path, []byte(b.String()))
}

// ParseFrontmatter parses a basic YAML frontmatter block and returns body content.
func ParseFrontmatter(content string) (map[string]interface{}, string, error) {
	if !strings.HasPrefix(content, "---\n") {
		return nil, "", fmt.Errorf("missing frontmatter header")
	}
	parts := strings.SplitN(content, "\n---\n", 2)
	if len(parts) != 2 {
		return nil, "", fmt.Errorf("invalid frontmatter delimiters")
	}
	raw := strings.TrimPrefix(parts[0], "---\n")
	body := parts[1]
	fm := map[string]interface{}{}
	lines := strings.Split(raw, "\n")
	var currentKey string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") && currentKey != "" {
			existing := toStringSlice(fm[currentKey])
			existing = append(existing, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
			fm[currentKey] = existing
			continue
		}
		if strings.Contains(trimmed, ":") {
			parts := strings.SplitN(trimmed, ":", 2)
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			currentKey = key
			if val == "[]" {
				fm[key] = []string{}
				continue
			}
			if val == "true" {
				fm[key] = true
				continue
			}
			if val == "false" {
				fm[key] = false
				continue
			}
			if val == "" {
				fm[key] = []string{}
				continue
			}
			fm[key] = val
		}
	}
	return fm, body, nil
}

// RenderFrontmatter renders a parsed frontmatter map and markdown body.
func RenderFrontmatter(frontmatter map[string]interface{}, body string) (string, error) {
	ordered := []string{"module", "root_path", "key_files", "depends_on", "changed_in", "generated_at", "id", "date", "type", "breaking", "modules"}
	var b strings.Builder
	b.WriteString("---\n")
	seen := map[string]struct{}{}
	for _, key := range ordered {
		v, ok := frontmatter[key]
		if !ok {
			continue
		}
		seen[key] = struct{}{}
		writeFMKey(&b, key, v)
	}
	for key, v := range frontmatter {
		if _, ok := seen[key]; ok {
			continue
		}
		writeFMKey(&b, key, v)
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimLeft(body, "\n"))
	return b.String(), nil
}

func writeFMKey(b *strings.Builder, key string, v interface{}) {
	switch val := v.(type) {
	case []string:
		if len(val) == 0 {
			b.WriteString(key + ": []\n")
			return
		}
		b.WriteString(key + ":\n")
		for _, item := range val {
			b.WriteString("  - " + item + "\n")
		}
	case []interface{}:
		items := toStringSlice(val)
		if len(items) == 0 {
			b.WriteString(key + ": []\n")
			return
		}
		b.WriteString(key + ":\n")
		for _, item := range items {
			b.WriteString("  - " + item + "\n")
		}
	case bool:
		b.WriteString(fmt.Sprintf("%s: %t\n", key, val))
	default:
		b.WriteString(fmt.Sprintf("%s: %v\n", key, val))
	}
}

func replaceSection(docBody, heading, replacement string) string {
	target := "## " + strings.TrimSpace(heading)
	lines := strings.Split(docBody, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == target {
			start = i
			break
		}
	}
	if start == -1 {
		trimmed := strings.TrimRight(docBody, "\n")
		return trimmed + "\n\n" + target + "\n\n" + strings.TrimSpace(replacement) + "\n"
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "## ") {
			end = i
			break
		}
	}
	updated := append([]string{}, lines[:start]...)
	updated = append(updated, target, "", strings.TrimSpace(replacement), "")
	updated = append(updated, lines[end:]...)
	return strings.Join(updated, "\n")
}

func toStringSlice(v interface{}) []string {
	switch value := v.(type) {
	case nil:
		return []string{}
	case []string:
		return append([]string(nil), value...)
	case []interface{}:
		out := make([]string, 0, len(value))
		for _, item := range value {
			out = append(out, fmt.Sprintf("%v", item))
		}
		return out
	default:
		if s := strings.TrimSpace(fmt.Sprintf("%v", value)); s != "" {
			return []string{s}
		}
		return []string{}
	}
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func ensureChangelogSection(body string, changeIDs []string, latestID, latestSummary string) string {
	target := "## Changelog"
	lines := strings.Split(body, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == target {
			start = i
			break
		}
	}
	end := len(lines)
	if start != -1 {
		for i := start + 1; i < len(lines); i++ {
			if strings.HasPrefix(strings.TrimSpace(lines[i]), "## ") {
				end = i
				break
			}
		}
	}

	entries := map[string]string{}
	if start != -1 {
		for _, line := range lines[start+1 : end] {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "- `") {
				continue
			}
			idStart := strings.Index(trimmed, "`")
			idEnd := strings.Index(trimmed[idStart+1:], "`")
			if idStart == -1 || idEnd == -1 {
				continue
			}
			id := trimmed[idStart+1 : idStart+1+idEnd]
			desc := ""
			if sep := strings.Index(trimmed, " - "); sep != -1 {
				desc = strings.TrimSpace(trimmed[sep+3:])
			}
			if id != "" {
				entries[id] = desc
			}
		}
	}

	if strings.TrimSpace(latestID) != "" {
		summary := strings.TrimSpace(latestSummary)
		if summary == "" {
			summary = "See .devmem/changelog/" + latestID + ".md"
		}
		entries[latestID] = summary
	}

	built := make([]string, 0, len(changeIDs))
	for _, id := range changeIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		desc := strings.TrimSpace(entries[id])
		if desc == "" {
			desc = "See .devmem/changelog/" + id + ".md"
		}
		built = append(built, fmt.Sprintf("- `%s` - %s", id, desc))
	}
	if len(built) == 0 {
		built = append(built, "No changes captured yet. Run devmem capture after your next commit.")
	}
	sectionBody := strings.Join(built, "\n")

	if start == -1 {
		trimmed := strings.TrimRight(body, "\n")
		return trimmed + "\n\n## Changelog\n\n" + sectionBody + "\n"
	}
	updated := append([]string{}, lines[:start]...)
	updated = append(updated, target, "", sectionBody, "")
	updated = append(updated, lines[end:]...)
	return strings.Join(updated, "\n")
}

func atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}
