/*
Copyright © 2026 Surya Parida

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/Surya-sourav/devmem/internal/ai"
	"github.com/Surya-sourav/devmem/internal/state"
	"github.com/Surya-sourav/devmem/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	repoDir  string
	model    string
	provider string
	apiKey   string
	keyFrom  string

	providerFromFlag bool
	modelFromFlag    bool
)

var rootCmd = &cobra.Command{
	Use:           "devmem",
	Short:         "Persistent AI memory for codebases",
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if repoDir == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("determine working directory: %w", err)
			}
			repoDir = cwd
		}
		abs, err := filepath.Abs(repoDir)
		if err != nil {
			return fmt.Errorf("resolve --dir: %w", err)
		}
		repoDir = abs

		cfg, err := state.LoadConfig(repoDir)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		providerFromFlag = cmd.Flags().Changed("provider")
		modelFromFlag = cmd.Flags().Changed("model")
		if !providerFromFlag {
			if env := os.Getenv("DEVMEM_PROVIDER"); env != "" {
				provider = env
				providerFromFlag = true
			} else {
				provider = cfg.Provider
			}
		}
		provider = strings.ToLower(strings.TrimSpace(provider))
		if !ai.ValidProvider(provider) {
			return fmt.Errorf("unknown provider %q; use anthropic or openai", provider)
		}
		return selectProvider(provider, cfg)
	},
}

// selectProvider points the global key and model at the given provider.
func selectProvider(p string, cfg *state.Config) error {
	provider = p
	if !modelFromFlag {
		model = cfg.AIModel
		if model == "" || cfg.Provider != p {
			model = ai.DefaultModel(p)
		}
	}
	apiKey, keyFrom = "", ""
	for _, name := range keyEnvVars(p) {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			apiKey, keyFrom = v, name
			return nil
		}
	}
	persisted, err := state.LoadAPIKey(repoDir, p)
	if err != nil {
		return fmt.Errorf("load persisted api key: %w", err)
	}
	if persisted != "" {
		apiKey, keyFrom = persisted, "persisted"
	}
	return nil
}

func keyEnvVars(p string) []string {
	if p == ai.ProviderOpenAI {
		return []string{"DEVMEM_API_KEY", "OPENAI_API_KEY"}
	}
	return []string{"DEVMEM_API_KEY", "ANTHROPIC_API_KEY"}
}

// Execute runs the root command. The context is cancelled on Ctrl+C so
// in-flight API calls stop instead of leaving half-written docs behind.
func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		stop()
	}()
	err := rootCmd.ExecuteContext(ctx)
	if err != nil && ctx.Err() != nil {
		return ui.ErrCancelled
	}
	return err
}

func init() {
	cwd, _ := os.Getwd()
	rootCmd.PersistentFlags().StringVar(&repoDir, "dir", cwd, "Repository root directory")
	rootCmd.PersistentFlags().StringVar(&provider, "provider", "", "AI provider: anthropic or openai (default from config)")
	rootCmd.PersistentFlags().StringVar(&model, "model", "", "AI model (default depends on provider)")
	viper.SetConfigName("config")
	viper.SetConfigType("json")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()
	_ = viper.ReadInConfig()
}
