/*
Copyright © 2026 Surya Parida <surya.aimlx@gmail.com>
*/
package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"github.com/yourusername/devmem/internal/git"
	"github.com/yourusername/devmem/internal/state"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show DevMem documentation status",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := state.LoadState(repoDir)
		if err != nil {
			return fmt.Errorf("load state: %w", err)
		}
		cfg, err := state.LoadConfig(repoDir)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		head, err := git.GetCurrentCommit(repoDir)
		if err != nil {
			return fmt.Errorf("get current commit: %w", err)
		}
		if head == s.LastCommit {
			fmt.Fprintln(os.Stdout, "All documentation is current.")
			return nil
		}
		files, err := git.GetChangedFiles(repoDir, s.LastCommit, head)
		if err != nil {
			return fmt.Errorf("get changed files: %w", err)
		}
		affected := modulesForFiles(cfg, files)
		names := keys(affected)
		sort.Strings(names)
		fmt.Fprintln(os.Stdout, "Module\tStatus")
		for _, name := range names {
			fmt.Fprintf(os.Stdout, "%s\tpending-doc-update\n", name)
		}
		if len(names) == 0 {
			fmt.Fprintln(os.Stdout, "No module-level documentation updates required.")
			return nil
		}
		return fmt.Errorf("undocumented changes detected")
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
