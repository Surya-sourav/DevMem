/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yourusername/devmem/internal/ai"
	"github.com/yourusername/devmem/internal/docs"
	"github.com/yourusername/devmem/internal/git"
	"github.com/yourusername/devmem/internal/state"
)

var captureCmd = &cobra.Command{
	Use:   "capture",
	Short: "Capture git changes into DevMem docs",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAPIKey("capture"); err != nil {
			return err
		}
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		cfg, err := state.LoadConfig(repoDir)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		s, err := state.LoadState(repoDir)
		if err != nil {
			return fmt.Errorf("load state: %w", err)
		}
		toCommit, err := git.GetCurrentCommit(repoDir)
		if err != nil {
			return fmt.Errorf("get current commit: %w", err)
		}
		fromCommit := s.LastCommit
		if strings.TrimSpace(fromCommit) == "" {
			fromCommit = "HEAD~1"
		}
		usingWorktree := false
		files, err := git.GetChangedFiles(repoDir, fromCommit, toCommit)
		if err != nil {
			return fmt.Errorf("get changed files: %w", err)
		}
		patch := ""
		if len(files) > 0 {
			patch, err = git.GetDiffPatch(repoDir, fromCommit, toCommit)
			if err != nil {
				return fmt.Errorf("get diff patch: %w", err)
			}
		} else {
			files, err = git.GetChangedFilesFromWorktree(repoDir)
			if err != nil {
				return fmt.Errorf("get worktree changed files: %w", err)
			}
			if len(files) > 0 {
				usingWorktree = true
				patch, err = git.GetWorktreeDiffPatch(repoDir)
				if err != nil {
					return fmt.Errorf("get worktree diff patch: %w", err)
				}
			}
		}
		affected := modulesForFiles(cfg, files)
		if len(affected) == 0 {
			fmt.Fprintln(os.Stderr, "No module-affecting changes detected.")
			if !usingWorktree {
				s.LastCommit = toCommit
			}
			s.LastCapture = time.Now().UTC().Format(time.RFC3339)
			return state.WriteState(repoDir, s)
		}
		changeID := toCommit
		if usingWorktree {
			changeID = fmt.Sprintf("%s-worktree-%s", shortCommit(toCommit), time.Now().UTC().Format("20060102T150405Z"))
		}
		client := ai.NewClient(apiKey, model)
		classification, err := client.ClassifyChange(ctx, ai.ChangePromptInput{
			CommitMessage: changeID,
			DiffPatch:     patch,
			ModuleNames:   keys(affected),
		})
		if err != nil {
			return fmt.Errorf("classify change: %w", err)
		}
		for moduleName := range affected {
			docPath := filepath.Join(repoDir, ".devmem", "docs", "modules", moduleName+".md")
			current, readErr := os.ReadFile(docPath)
			if readErr != nil {
				return fmt.Errorf("read module doc %s: %w", moduleName, readErr)
			}
			moduleSummary := classification.Modules[moduleName]
			if moduleSummary == "" {
				moduleSummary = classification.Summary
			}
			docPatch, patchErr := client.PatchModuleDoc(ctx, ai.PatchPromptInput{
				CurrentDoc:    string(current),
				ChangeSummary: moduleSummary,
				DiffPatch:     patch,
			})
			if patchErr != nil {
				return fmt.Errorf("patch module doc for %s: %w", moduleName, patchErr)
			}
			if writeErr := docs.PatchModuleDoc(repoRootOrDefault(repoDir), moduleName, *docPatch, changeID); writeErr != nil {
				return fmt.Errorf("write patched module doc for %s: %w", moduleName, writeErr)
			}
		}
		if err := docs.WriteChangelogEntry(repoDir, changeID, *classification, time.Now().UTC()); err != nil {
			return fmt.Errorf("write changelog entry: %w", err)
		}
		if classification.Type == "structural" {
			analyses, loadErr := loadAnalysesFromModuleDocs(repoDir)
			if loadErr != nil {
				return fmt.Errorf("load module analyses for structural refresh: %w", loadErr)
			}
			master, genErr := client.GenerateMaster(ctx, ai.MasterPromptInput{
				ProjectName: filepath.Base(repoDir),
				Modules:     analyses,
			})
			if genErr != nil {
				return fmt.Errorf("regenerate master architecture: %w", genErr)
			}
			if writeErr := docs.WriteMasterDoc(repoDir, *master, time.Now().UTC()); writeErr != nil {
				return fmt.Errorf("write master architecture markdown: %w", writeErr)
			}
			if writeErr := docs.WriteMermaid(repoDir, *master); writeErr != nil {
				return fmt.Errorf("write master architecture mermaid: %w", writeErr)
			}
		}
		if !usingWorktree {
			s.LastCommit = toCommit
		}
		s.LastCapture = time.Now().UTC().Format(time.RFC3339)
		if err := state.WriteState(repoDir, s); err != nil {
			return fmt.Errorf("write state: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Captured changes for %d module(s).\n", len(affected))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(captureCmd)
}

func modulesForFiles(cfg *state.Config, files []string) map[string]struct{} {
	affected := map[string]struct{}{}
	for _, file := range files {
		clean := filepath.ToSlash(strings.TrimSpace(file))
		for moduleName, mod := range cfg.Modules {
			for _, root := range mod.Paths {
				root = filepath.ToSlash(strings.TrimSpace(root))
				if clean == root || strings.HasPrefix(clean, root+"/") {
					affected[moduleName] = struct{}{}
				}
			}
		}
	}
	return affected
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func repoRootOrDefault(path string) string {
	if path != "" {
		return path
	}
	wd, _ := os.Getwd()
	return wd
}

func loadAnalysesFromModuleDocs(repoRoot string) ([]ai.ModuleAnalysis, error) {
	moduleDir := filepath.Join(repoRoot, ".devmem", "docs", "modules")
	entries, err := os.ReadDir(moduleDir)
	if err != nil {
		return nil, fmt.Errorf("read module docs directory: %w", err)
	}
	out := make([]ai.ModuleAnalysis, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(moduleDir, entry.Name())
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("read module doc %s: %w", path, readErr)
		}
		fm, body, parseErr := docs.ParseFrontmatter(string(content))
		if parseErr != nil {
			return nil, fmt.Errorf("parse module doc frontmatter %s: %w", path, parseErr)
		}
		desc := firstParagraph(body)
		name := strings.TrimSuffix(entry.Name(), ".md")
		if v, ok := fm["module"]; ok {
			if s := strings.TrimSpace(fmt.Sprintf("%v", v)); s != "" {
				name = s
			}
		}
		depends := toStringSlice(fm["depends_on"])
		keyFiles := toStringSlice(fm["key_files"])
		out = append(out, ai.ModuleAnalysis{
			Name:        name,
			Description: desc,
			Purpose:     desc,
			KeyFiles:    keyFiles,
			DependsOn:   depends,
			PublicAPI:   []string{},
			Patterns:    []string{},
		})
	}
	return out, nil
}

func firstParagraph(s string) string {
	parts := strings.Split(strings.TrimSpace(s), "\n\n")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
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
		s := strings.TrimSpace(fmt.Sprintf("%v", value))
		if s == "" {
			return []string{}
		}
		return []string{s}
	}
}

func requireAPIKey(commandName string) error {
	if strings.TrimSpace(apiKey) != "" {
		return nil
	}
	return fmt.Errorf("anthropic API key is required for %s; set DEVMEM_API_KEY or run devmem init and save it", commandName)
}

func shortCommit(hash string) string {
	trimmed := strings.TrimSpace(hash)
	if len(trimmed) > 12 {
		return trimmed[:12]
	}
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}
