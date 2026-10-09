/*
Copyright © 2026 Surya Parida <surya.aimlx@gmail.com>
*/
package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Surya-sourav/devmem/internal/ai"
	"github.com/Surya-sourav/devmem/internal/crawler"
	"github.com/Surya-sourav/devmem/internal/docs"
	"github.com/Surya-sourav/devmem/internal/git"
	"github.com/Surya-sourav/devmem/internal/state"
	"github.com/Surya-sourav/devmem/internal/ui"
	"github.com/spf13/cobra"
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

		head, err := git.GetCurrentCommit(repoDir)
		if err != nil {
			return fmt.Errorf("get current commit: %w", err)
		}
		fromCommit := strings.TrimSpace(s.LastCommit)
		if fromCommit == "" {
			fromCommit = "HEAD~1"
		}

		ui.Header("capture", shortCommit(head))
		client := ai.NewClient(provider, apiKey, model)
		totalCaptured := 0

		var warnings []string
		err = ui.Step("Checking for new modules", func() (string, error) {
			added, warns, err := detectAndDocumentNewModules(ctx, client, cfg)
			warnings = warns
			if err != nil {
				return "", err
			}
			totalCaptured += added
			if added == 0 {
				return "none", nil
			}
			return plural(added, "module") + " documented", nil
		})
		for _, w := range warnings {
			ui.Warn(truncate(w, 400), "")
		}
		if err != nil {
			return err
		}

		commitFiles, err := git.GetChangedFiles(repoDir, fromCommit, head)
		if err != nil {
			return fmt.Errorf("get changed files: %w", err)
		}
		if len(commitFiles) > 0 {
			commitPatch, patchErr := git.GetDiffPatch(repoDir, fromCommit, head)
			if patchErr != nil {
				return fmt.Errorf("get diff patch: %w", patchErr)
			}
			label := "Capturing commit " + shortCommit(head)
			if err := captureStep(ctx, client, cfg, label, head, commitFiles, commitPatch, &totalCaptured); err != nil {
				return err
			}
		}

		worktreeFiles, err := git.GetChangedFilesFromWorktree(repoDir)
		if err != nil {
			return fmt.Errorf("get worktree changed files: %w", err)
		}
		if len(worktreeFiles) > 0 {
			worktreePatch, patchErr := git.GetWorktreeDiffPatch(repoDir)
			if patchErr != nil {
				return fmt.Errorf("get worktree diff patch: %w", patchErr)
			}
			worktreeID := fmt.Sprintf("%s-worktree-%s", shortCommit(head), time.Now().UTC().Format("20060102T150405Z"))
			if err := captureStep(ctx, client, cfg, "Capturing uncommitted changes", worktreeID, worktreeFiles, worktreePatch, &totalCaptured); err != nil {
				return err
			}
		}

		s.LastCapture = time.Now().UTC().Format(time.RFC3339)
		if len(commitFiles) > 0 {
			s.LastCommit = head
		}
		s.ModuleCount = len(cfg.Modules)
		if err := state.WriteState(repoDir, s); err != nil {
			return fmt.Errorf("write state: %w", err)
		}

		ui.Blank()
		if totalCaptured == 0 {
			ui.Info("No module-affecting changes detected.", "")
		} else {
			ui.Success(fmt.Sprintf("Captured changes for %s", plural(totalCaptured, "module")), "")
		}
		ui.Blank()
		return nil
	},
}

func captureStep(ctx context.Context, client *ai.Client, cfg *state.Config, label, changeID string, files []string, patch string, total *int) error {
	var structural bool
	err := ui.Step(label, func() (string, error) {
		captured, isStructural, err := processCaptureChange(ctx, client, cfg, changeID, files, patch)
		if err != nil {
			return "", err
		}
		structural = isStructural
		*total += captured
		if captured == 0 {
			return "no tracked modules touched", nil
		}
		return plural(captured, "module") + " updated", nil
	})
	if err != nil || !structural {
		return err
	}
	return ui.Step("Refreshing architecture overview", func() (string, error) {
		start := time.Now()
		if err := regenerateMaster(ctx, client); err != nil {
			return "", err
		}
		return elapsed(start), nil
	})
}

func init() {
	rootCmd.AddCommand(captureCmd)
}

func processCaptureChange(ctx context.Context, client *ai.Client, cfg *state.Config, changeID string, files []string, patch string) (capturedCount int, structural bool, err error) {
	affected := modulesForFiles(cfg, files)
	if len(affected) == 0 {
		return 0, false, nil
	}

	classification, err := client.ClassifyChange(ctx, ai.ChangePromptInput{
		CommitMessage: changeID,
		DiffPatch:     patch,
		ModuleNames:   keys(affected),
	})
	if err != nil {
		return 0, false, fmt.Errorf("classify change (%s): %w", changeID, err)
	}

	for moduleName := range affected {
		docPath := filepath.Join(repoDir, ".devmem", "docs", "modules", moduleName+".md")
		current, readErr := os.ReadFile(docPath)
		if readErr != nil {
			return 0, false, fmt.Errorf("read module doc %s: %w", moduleName, readErr)
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
			return 0, false, fmt.Errorf("patch module doc for %s: %w", moduleName, patchErr)
		}
		delete(docPatch.Sections, "Changelog")
		delete(docPatch.Sections, "changelog")
		if writeErr := docs.PatchModuleDoc(repoRootOrDefault(repoDir), moduleName, *docPatch, changeID, moduleSummary); writeErr != nil {
			return 0, false, fmt.Errorf("write patched module doc for %s: %w", moduleName, writeErr)
		}
	}

	if err := docs.WriteChangelogEntry(repoDir, changeID, *classification, time.Now().UTC()); err != nil {
		return 0, false, fmt.Errorf("write changelog entry (%s): %w", changeID, err)
	}

	return len(affected), classification.Type == "structural", nil
}

func regenerateMaster(ctx context.Context, client *ai.Client) error {
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
	return nil
}

func detectAndDocumentNewModules(ctx context.Context, client *ai.Client, cfg *state.Config) (int, []string, error) {
	w := &crawler.Walker{Root: repoDir, IgnoreRules: ignoreRules(cfg)}
	tree, err := w.Walk()
	if err != nil {
		return 0, nil, fmt.Errorf("crawl repository for module detection: %w", err)
	}
	if err := state.WriteTree(repoDir, tree); err != nil {
		return 0, nil, fmt.Errorf("write tree snapshot: %w", err)
	}

	scorer := &crawler.ModuleScorer{Tree: tree, Config: nil}
	detected := scorer.Detect()
	newModules := make([]crawler.Module, 0)
	for _, mod := range detected {
		if !modulePathInConfig(cfg, mod.RootPath) {
			newModules = append(newModules, mod)
		}
	}
	if len(newModules) == 0 {
		return 0, nil, nil
	}

	sort.Slice(newModules, func(i, j int) bool {
		return newModules[i].Name < newModules[j].Name
	})

	moduleNames := make([]string, 0, len(cfg.Modules)+len(newModules))
	for name := range cfg.Modules {
		moduleNames = append(moduleNames, name)
	}
	for _, mod := range newModules {
		moduleNames = append(moduleNames, mod.Name)
	}

	added := 0
	var warnings []string
	for _, mod := range newModules {
		analysis, analysisErr := analyseOneModule(ctx, repoDir, client, mod, tree, moduleNames)
		if analysisErr != nil {
			warnings = append(warnings, fmt.Sprintf("Skipped new module %s: %v", mod.Name, analysisErr))
			continue
		}
		if writeErr := docs.WriteModuleDoc(repoDir, mod, *analysis); writeErr != nil {
			warnings = append(warnings, fmt.Sprintf("Skipped new module %s: %v", mod.Name, writeErr))
			continue
		}
		name := uniqueModuleName(cfg, mod.Name)
		cfg.Modules[name] = state.ModuleConfig{Paths: []string{mod.RootPath}}
		added++
	}
	if added == 0 {
		return 0, warnings, nil
	}

	if err := state.WriteConfig(repoDir, cfg); err != nil {
		return 0, nil, fmt.Errorf("write config with new modules: %w", err)
	}
	if err := regenerateMaster(ctx, client); err != nil {
		return 0, warnings, err
	}
	return added, warnings, nil
}

func modulePathInConfig(cfg *state.Config, candidatePath string) bool {
	candidatePath = filepath.ToSlash(strings.TrimSpace(candidatePath))
	for _, mod := range cfg.Modules {
		for _, p := range mod.Paths {
			p = filepath.ToSlash(strings.TrimSpace(p))
			if candidatePath == p {
				return true
			}
		}
	}
	return false
}

func uniqueModuleName(cfg *state.Config, base string) string {
	name := strings.TrimSpace(base)
	if name == "" {
		name = "module"
	}
	if _, exists := cfg.Modules[name]; !exists {
		return name
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", name, i)
		if _, exists := cfg.Modules[candidate]; !exists {
			return candidate
		}
	}
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
	return fmt.Errorf("no %s API key found for %s\nset %s or run devmem init and save the key",
		ai.ProviderLabel(provider), commandName, strings.Join(keyEnvVars(provider), " or "))
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
