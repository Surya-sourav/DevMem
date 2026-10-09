/*
Copyright © 2026 Surya Parida <surya.aimlx@gmail.com>
*/
package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"charm.land/huh/v2"
	"github.com/Surya-sourav/devmem/internal/ai"
	"github.com/Surya-sourav/devmem/internal/crawler"
	"github.com/Surya-sourav/devmem/internal/docs"
	"github.com/Surya-sourav/devmem/internal/git"
	"github.com/Surya-sourav/devmem/internal/state"
	"github.com/Surya-sourav/devmem/internal/ui"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialise DevMem for this repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		ui.Header("init", repoDir)

		cfg, err := state.LoadConfig(repoDir)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		if cfg.MaxConcurrent <= 0 {
			cfg.MaxConcurrent = 5
		}
		if err := state.EnsureDirs(repoDir); err != nil {
			return fmt.Errorf("ensure directories: %w", err)
		}
		if err := setupProvider(cfg); err != nil {
			return err
		}
		cfg.Provider = provider
		cfg.AIModel = model
		if err := state.WriteConfig(repoDir, cfg); err != nil {
			return fmt.Errorf("write config: %w", err)
		}

		var tree *crawler.FileNode
		err = ui.Step("Crawling codebase", func() (string, error) {
			start := time.Now()
			w := &crawler.Walker{Root: repoDir, IgnoreRules: ignoreRules(cfg)}
			t, err := w.Walk()
			if err != nil {
				return "", fmt.Errorf("crawl repository: %w", err)
			}
			if err := state.WriteTree(repoDir, t); err != nil {
				return "", fmt.Errorf("write tree snapshot: %w", err)
			}
			tree = t
			return fmt.Sprintf("%s · %s", plural(crawler.CountFiles(t), "file"), elapsed(start)), nil
		})
		if err != nil {
			return err
		}

		scorer := &crawler.ModuleScorer{Tree: tree, Config: cfg}
		modules := scorer.Detect()
		if len(modules) == 0 {
			return fmt.Errorf("no modules were detected; add module mappings to .devmem/config.json and retry")
		}
		ui.Success("Detected modules", plural(len(modules), "candidate"))
		ui.Blank()
		printModules(modules)
		ui.Blank()

		confirmed, err := confirmModules(modules)
		if err != nil {
			return err
		}
		modules = confirmed

		client := ai.NewClient(provider, apiKey, model)
		ui.Info(
			fmt.Sprintf("Analysing %s with %s", plural(len(modules), "module"), ui.Accent.Render(model)),
			fmt.Sprintf("up to %d at a time", cfg.MaxConcurrent),
		)

		type analysisResult struct {
			module   crawler.Module
			analysis *ai.ModuleAnalysis
			err      error
			idx      int
			dur      time.Duration
		}

		moduleNames := make([]string, 0, len(modules))
		for _, mod := range modules {
			moduleNames = append(moduleNames, mod.Name)
		}
		tasks := ui.StartTasks(moduleNames...)
		results := make(chan analysisResult, len(modules))
		sem := make(chan struct{}, cfg.MaxConcurrent)
		var wg sync.WaitGroup

		for i, mod := range modules {
			wg.Add(1)
			go func(idx int, m crawler.Module) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				tasks.Running(idx, "analysing")
				start := time.Now()
				analysis, err := analyseOneModule(ctx, repoDir, client, m, tree, moduleNames)
				results <- analysisResult{module: m, analysis: analysis, err: err, idx: idx, dur: time.Since(start)}
			}(i, mod)
		}

		go func() {
			wg.Wait()
			close(results)
		}()

		success := make([]ai.ModuleAnalysis, 0, len(modules))
		var failures []moduleFailure
		for r := range results {
			if r.err == nil {
				r.err = docs.WriteModuleDoc(repoDir, r.module, *r.analysis)
			}
			if r.err != nil {
				failures = append(failures, moduleFailure{r.module.Name, r.err})
				tasks.Fail(r.idx, "failed after "+roundDuration(r.dur))
				continue
			}
			success = append(success, *r.analysis)
			tasks.Done(r.idx, roundDuration(r.dur))
		}
		tasks.Wait()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		printFailures(failures)
		if len(success) == 0 {
			return fmt.Errorf("no modules were documented successfully; see the errors above")
		}
		ui.Blank()

		err = ui.Step("Generating architecture overview", func() (string, error) {
			start := time.Now()
			master, err := client.GenerateMaster(ctx, ai.MasterPromptInput{
				ProjectName: filepath.Base(repoDir),
				Modules:     success,
			})
			if err != nil {
				return "", fmt.Errorf("generate master architecture: %w", err)
			}
			if err := docs.WriteMasterDoc(repoDir, *master, time.Now().UTC()); err != nil {
				return "", fmt.Errorf("write master architecture markdown: %w", err)
			}
			if err := docs.WriteMermaid(repoDir, *master); err != nil {
				return "", fmt.Errorf("write master architecture mermaid: %w", err)
			}
			return elapsed(start), nil
		})
		if err != nil {
			return err
		}

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

		summary := plural(len(success), "module") + " documented"
		if len(failures) > 0 {
			summary += ui.Muted.Render(fmt.Sprintf(" · %d failed", len(failures)))
		}
		ui.Blank()
		ui.Box("DevMem is ready",
			summary,
			"",
			ui.Muted.Render(".devmem/docs/modules/"),
			ui.Muted.Render(".devmem/docs/master-architecture.md"),
			ui.Muted.Render(".devmem/docs/master-architecture.mermaid"),
			ui.Muted.Render(".devmem/tree.json"),
			"",
			"Run "+ui.Code.Render("devmem capture")+" after a commit to keep docs current.",
		)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func printModules(modules []crawler.Module) {
	nameW, pathW := 0, 0
	for _, m := range modules {
		nameW = max(nameW, len(m.Name))
		pathW = max(pathW, len(m.RootPath)+1)
	}
	for _, m := range modules {
		name := fmt.Sprintf("%-*s", nameW, m.Name)
		path := fmt.Sprintf("%-*s", pathW, m.RootPath+"/")
		ui.Println("  " + ui.Accent.Render(name) + "  " + path + "  " + ui.Muted.Render(fmt.Sprintf("score %d · %s", m.Score, m.Source)))
	}
}

func confirmModules(modules []crawler.Module) ([]crawler.Module, error) {
	choice := "yes"
	err := ui.Ask(huh.NewSelect[string]().
		Title(fmt.Sprintf("Document these %s?", plural(len(modules), "module"))).
		Options(
			huh.NewOption("Yes, document them", "yes"),
			huh.NewOption("Edit the module list", "edit"),
			huh.NewOption("Cancel", "no"),
		).
		Value(&choice))
	if err != nil {
		return nil, err
	}
	switch choice {
	case "no":
		return nil, ui.ErrCancelled
	case "edit":
		var text string
		err := ui.Ask(huh.NewInput().
			Title("Modules to document").
			Description("Comma-separated name:path pairs. Leave empty to keep the detected list.").
			Placeholder("auth:src/auth, api:src/api").
			Value(&text).
			Validate(func(s string) error {
				_, err := parseModuleList(s)
				return err
			}))
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(text) == "" {
			return modules, nil
		}
		return parseModuleList(text)
	}
	return modules, nil
}

type moduleFailure struct {
	module string
	err    error
}

// printFailures groups modules that failed with the same error, which is
// the usual case for a bad key or an unknown model.
func printFailures(failures []moduleFailure) {
	if len(failures) == 0 {
		return
	}
	var order []string
	byErr := map[string][]string{}
	for _, f := range failures {
		msg := f.err.Error()
		if _, seen := byErr[msg]; !seen {
			order = append(order, msg)
		}
		byErr[msg] = append(byErr[msg], f.module)
	}
	ui.Blank()
	for _, msg := range order {
		ui.Warn(strings.Join(byErr[msg], ", "), "")
		ui.Println("  " + ui.Muted.Render(truncate(msg, 400)))
	}
}

func parseModuleList(text string) ([]crawler.Module, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	parts := strings.Split(text, ",")
	out := make([]crawler.Module, 0, len(parts))
	for _, part := range parts {
		pair := strings.SplitN(strings.TrimSpace(part), ":", 2)
		if len(pair) != 2 || strings.TrimSpace(pair[0]) == "" || strings.TrimSpace(pair[1]) == "" {
			return nil, fmt.Errorf("expected name:path, got %q", strings.TrimSpace(part))
		}
		name := strings.TrimSpace(pair[0])
		path := filepath.ToSlash(strings.TrimSpace(pair[1]))
		out = append(out, crawler.Module{Name: name, RootPath: path, Paths: []string{path}, Score: 99, Source: "config"})
	}
	return out, nil
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

// setupProvider asks which provider to use (unless --provider or
// DEVMEM_PROVIDER decided it) and makes sure there is a key for it.
func setupProvider(cfg *state.Config) error {
	if !providerFromFlag {
		choice := provider
		err := ui.Ask(huh.NewSelect[string]().
			Title("Which AI provider should DevMem use?").
			Options(
				huh.NewOption("Anthropic  (Claude)", ai.ProviderAnthropic),
				huh.NewOption("OpenAI     (GPT)", ai.ProviderOpenAI),
			).
			Value(&choice))
		if err != nil {
			return err
		}
		if err := selectProvider(choice, cfg); err != nil {
			return err
		}
	}
	label := ai.ProviderLabel(provider)

	if keyFrom == "persisted" {
		reuse := true
		if err := ui.Ask(huh.NewConfirm().
			Title(fmt.Sprintf("Use the saved %s key (%s)?", label, maskKey(apiKey))).
			Affirmative("Use it").
			Negative("Enter a new one").
			Value(&reuse)); err != nil {
			return err
		}
		if !reuse {
			apiKey, keyFrom = "", ""
		}
	}

	switch keyFrom {
	case "persisted":
		ui.Success(label+" API key", "loaded from .devmem/credentials.json")
	case "":
		key, err := promptAPIKey(label)
		if err != nil {
			return err
		}
		apiKey = key
		save := true
		if err := ui.Ask(huh.NewConfirm().
			Title("Save this key for future runs?").
			Description("Stored in .devmem/credentials.json, which you should keep out of git.").
			Affirmative("Save").
			Negative("Don't save").
			Value(&save)); err != nil {
			return err
		}
		if save {
			if err := state.SaveAPIKey(repoDir, provider, apiKey); err != nil {
				return fmt.Errorf("save api key: %w", err)
			}
			ui.Success(label+" API key", "saved to .devmem/credentials.json")
		} else {
			ui.Success(label+" API key", "used for this run only")
		}
	default:
		ui.Success(label+" API key", "from $"+keyFrom)
	}
	ui.Success("Model", model)
	return nil
}

func promptAPIKey(label string) (string, error) {
	where := "https://console.anthropic.com/settings/keys"
	if provider == ai.ProviderOpenAI {
		where = "https://platform.openai.com/api-keys"
	}
	var key string
	err := ui.Ask(ui.Secret(huh.NewInput()).
		Title(label + " API key").
		Description("Create one at " + where).
		Value(&key).
		Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("an API key is required")
			}
			return nil
		}))
	if err != nil {
		return "", err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("%s API key is required for init", label)
	}
	return key, nil
}

func maskKey(key string) string {
	if len(key) < 12 {
		return "…"
	}
	return key[:4] + "…" + key[len(key)-4:]
}

func ignoreRules(cfg *state.Config) []string {
	rules := append([]string{}, cfg.Ignore...)
	rules = append(rules, crawler.LoadIgnoreFile(filepath.Join(repoDir, ".gitignore"))...)
	rules = append(rules, crawler.LoadIgnoreFile(filepath.Join(repoDir, ".devmemignore"))...)
	return rules
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func elapsed(start time.Time) string {
	return roundDuration(time.Since(start))
}

func roundDuration(d time.Duration) string {
	if d < time.Second {
		return d.Round(time.Millisecond).String()
	}
	return d.Round(100 * time.Millisecond).String()
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
