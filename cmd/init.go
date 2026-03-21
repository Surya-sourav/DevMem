/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/yourusername/devmem/internal/ai"
	"github.com/yourusername/devmem/internal/crawler"
	"github.com/yourusername/devmem/internal/docs"
	"github.com/yourusername/devmem/internal/git"
	"github.com/yourusername/devmem/internal/state"
)

type Progress struct {
	mu sync.Mutex
}

func (p *Progress) Printf(format string, args ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintf(os.Stderr, format, args...)
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialise DevMem for this repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		progress := &Progress{}
		progress.Printf("devmem init\n")

		cfg, err := state.LoadConfig(repoDir)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		cfg.AIModel = model
		if cfg.MaxConcurrent <= 0 {
			cfg.MaxConcurrent = 5
		}

		if err := state.EnsureDirs(repoDir); err != nil {
			return fmt.Errorf("ensure directories: %w", err)
		}
		if err := ensureInitAPIKey(); err != nil {
			return err
		}

		phaseStart := time.Now()
		progress.Printf("  Crawling codebase...           ")
		ignoreRules := append([]string{}, cfg.Ignore...)
		ignoreRules = append(ignoreRules, crawler.LoadIgnoreFile(filepath.Join(repoDir, ".gitignore"))...)
		ignoreRules = append(ignoreRules, crawler.LoadIgnoreFile(filepath.Join(repoDir, ".devmemignore"))...)
		w := &crawler.Walker{Root: repoDir, IgnoreRules: ignoreRules}
		tree, err := w.Walk()
		if err != nil {
			return fmt.Errorf("crawl repository: %w", err)
		}
		if err := state.WriteTree(repoDir, tree); err != nil {
			return fmt.Errorf("write tree snapshot: %w", err)
		}
		fileCount := crawler.CountFiles(tree)
		progress.Printf("done  (%d files, %.1fs)\n", fileCount, time.Since(phaseStart).Seconds())

		progress.Printf("  Detecting modules...           ")
		scorer := &crawler.ModuleScorer{Tree: tree, Config: cfg}
		modules := scorer.Detect()
		progress.Printf("found %d candidates\n\n", len(modules))
		if len(modules) == 0 {
			return fmt.Errorf("no modules were detected; add module mappings to .devmem/config.json and retry")
		}

		progress.Printf("  Detected modules:\n")
		for _, m := range modules {
			progress.Printf("    %-10s %-24s (score: %d, %s)\n", m.Name, m.RootPath+"/", m.Score, m.Source)
		}
		progress.Printf("\n")

		confirmed, err := confirmModules(modules)
		if err != nil {
			return err
		}
		modules = confirmed

		client := ai.NewClient(apiKey, model)
		progress.Printf("  Analysing modules (%d total, max %d parallel)...\n", len(modules), cfg.MaxConcurrent)

		type analysisResult struct {
			module   crawler.Module
			analysis *ai.ModuleAnalysis
			err      error
			idx      int
			dur      time.Duration
		}

		results := make(chan analysisResult, len(modules))
		sem := make(chan struct{}, cfg.MaxConcurrent)
		var wg sync.WaitGroup
		moduleNames := make([]string, 0, len(modules))
		for _, mod := range modules {
			moduleNames = append(moduleNames, mod.Name)
		}

		for i, mod := range modules {
			wg.Add(1)
			go func(idx int, m crawler.Module) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				start := time.Now()
				analysis, err := analyseOneModule(ctx, repoDir, client, m, tree, moduleNames)
				results <- analysisResult{module: m, analysis: analysis, err: err, idx: idx + 1, dur: time.Since(start)}
			}(i, mod)
		}

		go func() {
			wg.Wait()
			close(results)
		}()

		success := make([]ai.ModuleAnalysis, 0, len(modules))
		failed := 0
		for r := range results {
			if r.err != nil {
				failed++
				progress.Printf("    [%d/%d] %-10s failed (%s)\n", r.idx, len(modules), r.module.Name, r.dur.Round(100*time.Millisecond))
				progress.Printf("      error: %v\n", r.err)
				continue
			}
			if err := docs.WriteModuleDoc(repoDir, r.module, *r.analysis); err != nil {
				failed++
				progress.Printf("    [%d/%d] %-10s failed (%s)\n", r.idx, len(modules), r.module.Name, r.dur.Round(100*time.Millisecond))
				progress.Printf("      error: %v\n", err)
				continue
			}
			success = append(success, *r.analysis)
			progress.Printf("    [%d/%d] %-10s done  (%s)\n", r.idx, len(modules), r.module.Name, r.dur.Round(100*time.Millisecond))
		}
		if len(success) == 0 {
			return fmt.Errorf("no modules were documented successfully; %d failed, check module error logs above", failed)
		}

		progress.Printf("\n")
		progress.Printf("  Generating master architecture...  ")
		masterStart := time.Now()
		master, err := client.GenerateMaster(ctx, ai.MasterPromptInput{
			ProjectName: filepath.Base(repoDir),
			Modules:     success,
		})
		if err != nil {
			return fmt.Errorf("generate master architecture: %w", err)
		}
		if err := docs.WriteMasterDoc(repoDir, *master, time.Now().UTC()); err != nil {
			return fmt.Errorf("write master architecture markdown: %w", err)
		}
		if err := docs.WriteMermaid(repoDir, *master); err != nil {
			return fmt.Errorf("write master architecture mermaid: %w", err)
		}
		progress.Printf("done  (%.1fs)\n\n", time.Since(masterStart).Seconds())

		head, err := git.GetCurrentCommit(repoDir)
		if err != nil {
			return fmt.Errorf("get git HEAD: %w", err)
		}
		s := &state.State{
			InitialisedAt: time.Now().UTC().Format(time.RFC3339),
			LastCommit:    head,
			ModuleCount:   len(success),
		}
		if err := state.WriteState(repoDir, s); err != nil {
			return fmt.Errorf("write state: %w", err)
		}
		if len(cfg.Modules) == 0 {
			cfg.Modules = map[string]state.ModuleConfig{}
			for _, m := range modules {
				cfg.Modules[m.Name] = state.ModuleConfig{Paths: []string{m.RootPath}}
			}
			if err := state.WriteConfig(repoDir, cfg); err != nil {
				return fmt.Errorf("write config: %w", err)
			}
		}

		progress.Printf("  devmem initialised successfully.\n")
		progress.Printf("    %d modules documented successfully, %d failed\n", len(success), failed)
		progress.Printf("    .devmem/docs/modules/\n")
		progress.Printf("    .devmem/docs/master-architecture.md\n")
		progress.Printf("    .devmem/docs/master-architecture.mermaid\n")
		progress.Printf("    .devmem/tree.json\n")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func confirmModules(modules []crawler.Module) ([]crawler.Module, error) {
	reader := bufio.NewScanner(os.Stdin)
	fmt.Fprint(os.Stderr, "  Confirm? [Y/n/edit]: ")
	if !reader.Scan() {
		return modules, nil
	}
	choice := strings.TrimSpace(strings.ToLower(reader.Text()))
	switch choice {
	case "", "y", "yes":
		return modules, nil
	case "n", "no":
		return nil, fmt.Errorf("module confirmation declined")
	case "edit":
		fmt.Fprintln(os.Stderr, "  Enter modules as name:path pairs separated by commas (example: auth:src/auth,api:src/api):")
		fmt.Fprint(os.Stderr, "  > ")
		if !reader.Scan() {
			return nil, fmt.Errorf("no edit input provided")
		}
		text := strings.TrimSpace(reader.Text())
		if text == "" {
			return modules, nil
		}
		parts := strings.Split(text, ",")
		out := make([]crawler.Module, 0, len(parts))
		for _, part := range parts {
			pair := strings.SplitN(strings.TrimSpace(part), ":", 2)
			if len(pair) != 2 {
				return nil, fmt.Errorf("invalid module edit entry: %s", part)
			}
			name := strings.TrimSpace(pair[0])
			path := filepath.ToSlash(strings.TrimSpace(pair[1]))
			out = append(out, crawler.Module{Name: name, RootPath: path, Paths: []string{path}, Score: 99, Source: "config"})
		}
		return out, nil
	default:
		return nil, fmt.Errorf("invalid confirmation value: %s", choice)
	}
}

func analyseOneModule(ctx context.Context, repoRoot string, client *ai.Client, mod crawler.Module, tree *crawler.FileNode, moduleNames []string) (*ai.ModuleAnalysis, error) {
	node := crawler.FindNode(tree, mod.RootPath)
	if node == nil {
		return nil, fmt.Errorf("module root path not found in tree: %s", mod.RootPath)
	}
	entryContent := ""
	if mod.EntryFile != "" {
		entryPath := filepath.Join(repoRoot, filepath.FromSlash(mod.EntryFile))
		if b, err := os.ReadFile(entryPath); err == nil {
			lines := strings.Split(string(b), "\n")
			if len(lines) > 120 {
				lines = lines[:120]
			}
			entryContent = strings.Join(lines, "\n")
		}
	}
	input := ai.ModulePromptInput{
		ModuleName:   mod.Name,
		FileTree:     crawler.TreeToString(node, 0),
		EntryContent: entryContent,
		SiblingNames: moduleNames,
	}
	return client.AnalyseModule(ctx, input)
}

func ensureInitAPIKey() error {
	stored, err := state.LoadAPIKey(repoDir)
	if err != nil {
		return fmt.Errorf("load persisted api key: %w", err)
	}
	reader := bufio.NewScanner(os.Stdin)

	if strings.TrimSpace(apiKey) == "" {
		fmt.Fprint(os.Stderr, "  Enter Anthropic API key: ")
		if !reader.Scan() {
			return fmt.Errorf("anthropic api key is required for init")
		}
		apiKey = strings.TrimSpace(reader.Text())
		if apiKey == "" {
			return fmt.Errorf("anthropic api key is required for init")
		}
		fmt.Fprint(os.Stderr, "  Save API key for future runs? [Y/n]: ")
		if !reader.Scan() {
			return nil
		}
		choice := strings.TrimSpace(strings.ToLower(reader.Text()))
		if choice == "" || choice == "y" || choice == "yes" {
			if err := state.SaveAPIKey(repoDir, apiKey); err != nil {
				return fmt.Errorf("save api key: %w", err)
			}
			fmt.Fprintln(os.Stderr, "  API key saved to .devmem/credentials.json")
		}
		return nil
	}

	if keyFrom == "env" && strings.TrimSpace(stored) == "" {
		fmt.Fprint(os.Stderr, "  Save DEVMEM_API_KEY to .devmem/credentials.json for future runs? [y/N]: ")
		if !reader.Scan() {
			return nil
		}
		choice := strings.TrimSpace(strings.ToLower(reader.Text()))
		if choice == "y" || choice == "yes" {
			if err := state.SaveAPIKey(repoDir, apiKey); err != nil {
				return fmt.Errorf("save api key: %w", err)
			}
			fmt.Fprintln(os.Stderr, "  API key saved to .devmem/credentials.json")
		}
	}

	return nil
}
