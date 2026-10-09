/*
Copyright © 2026 Surya Parida <surya.aimlx@gmail.com>
*/
package cmd

import (
	"errors"
	"fmt"
	"sort"

	"github.com/Surya-sourav/devmem/internal/git"
	"github.com/Surya-sourav/devmem/internal/state"
	"github.com/Surya-sourav/devmem/internal/ui"
	"github.com/spf13/cobra"
)

// ErrPendingDocs makes status exit non-zero when docs are behind, so it
// can gate CI. The pending list has already been printed.
var ErrPendingDocs = errors.New("undocumented changes detected")

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
			ui.Stdout("  " + ui.OK() + " All documentation is current " + ui.Muted.Render("("+shortCommit(head)+")"))
			return nil
		}
		files, err := git.GetChangedFiles(repoDir, s.LastCommit, head)
		if err != nil {
			return fmt.Errorf("get changed files: %w", err)
		}
		affected := modulesForFiles(cfg, files)
		names := keys(affected)
		sort.Strings(names)
		if len(names) == 0 {
			ui.Stdout("  " + ui.OK() + " No module-level documentation updates required " +
				ui.Muted.Render(fmt.Sprintf("(%s since %s)", plural(len(files), "file"), shortCommit(s.LastCommit))))
			return nil
		}
		ui.Stdout("  " + ui.Bold.Render("Pending documentation updates") + " " +
			ui.Muted.Render(fmt.Sprintf("since %s", shortCommit(s.LastCommit))))
		ui.Stdout("")
		for _, name := range names {
			ui.Stdout("  " + ui.Pending() + " " + name)
		}
		ui.Stdout("")
		ui.Stdout("  " + ui.Muted.Render("Run ") + ui.Code.Render("devmem capture") + ui.Muted.Render(" to update them."))
		return ErrPendingDocs
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
