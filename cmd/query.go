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

	"github.com/spf13/cobra"
	"github.com/yourusername/devmem/internal/ai"
)

var queryCmd = &cobra.Command{
	Use:   "query <question>",
	Short: "Ask a grounded question over generated docs",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAPIKey("query"); err != nil {
			return err
		}
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		question := strings.TrimSpace(args[0])
		if question == "" {
			return fmt.Errorf("question cannot be empty")
		}
		contextDocs, err := loadAllDocs(repoDir)
		if err != nil {
			return err
		}
		client := ai.NewClient(apiKey, model)
		system := "Answer the user's question using only the provided DevMem documentation context. If unsupported by context, say so clearly."
		user := "Documentation context:\n" + contextDocs + "\n\nQuestion:\n" + question
		answer, err := client.Call(ctx, system, user, 1400)
		if err != nil {
			return fmt.Errorf("query AI: %w", err)
		}
		fmt.Fprintln(os.Stdout, answer)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(queryCmd)
}

func loadAllDocs(repoRoot string) (string, error) {
	modulesDir := filepath.Join(repoRoot, ".devmem", "docs", "modules")
	entries, err := os.ReadDir(modulesDir)
	if err != nil {
		return "", fmt.Errorf("read modules docs directory: %w", err)
	}
	var b strings.Builder
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		p := filepath.Join(modulesDir, e.Name())
		content, readErr := os.ReadFile(p)
		if readErr != nil {
			return "", fmt.Errorf("read module doc %s: %w", p, readErr)
		}
		b.WriteString("\n\n# FILE: " + e.Name() + "\n")
		b.Write(content)
	}
	masterPath := filepath.Join(repoRoot, ".devmem", "docs", "master-architecture.md")
	if master, readErr := os.ReadFile(masterPath); readErr == nil {
		b.WriteString("\n\n# FILE: master-architecture.md\n")
		b.Write(master)
	}
	return b.String(), nil
}
