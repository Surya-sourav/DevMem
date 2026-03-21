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
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/yourusername/devmem/internal/state"
)

var (
	repoDir string
	model   string
	apiKey  string
	keyFrom string
)

var rootCmd = &cobra.Command{
	Use:   "devmem",
	Short: "Persistent AI memory for codebases",
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
		apiKey = os.Getenv("DEVMEM_API_KEY")
		keyFrom = ""
		if apiKey != "" {
			keyFrom = "env"
		} else {
			persisted, loadErr := state.LoadAPIKey(repoDir)
			if loadErr != nil {
				return fmt.Errorf("load persisted api key: %w", loadErr)
			}
			if persisted != "" {
				apiKey = persisted
				keyFrom = "persisted"
			}
		}
		if model == "" {
			model = "claude-sonnet-4-20250514"
		}
		return nil
	},
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cwd, _ := os.Getwd()
	rootCmd.PersistentFlags().StringVar(&repoDir, "dir", cwd, "Repository root directory")
	rootCmd.PersistentFlags().StringVar(&model, "model", "claude-sonnet-4-20250514", "AI model")
	viper.SetConfigName("config")
	viper.SetConfigType("json")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()
	_ = viper.ReadInConfig()
}
