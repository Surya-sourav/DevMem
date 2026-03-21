package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const hookLine = "devmem capture"

// InstallPostCommitHook installs or updates .git/hooks/post-commit with a devmem capture call.
func InstallPostCommitHook(repoRoot string) error {
	hookPath := filepath.Join(repoRoot, ".git", "hooks", "post-commit")
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		return fmt.Errorf("create hooks directory: %w", err)
	}
	content := "#!/bin/sh\n"
	existing, err := os.ReadFile(hookPath)
	if err == nil {
		content = string(existing)
		if strings.Contains(content, hookLine) {
			if chmodErr := os.Chmod(hookPath, 0o755); chmodErr != nil {
				return fmt.Errorf("set hook permissions: %w", chmodErr)
			}
			return nil
		}
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
	}
	content += "cd \"$(git rev-parse --show-toplevel)\" && " + hookLine + "\n"
	if err := os.WriteFile(hookPath, []byte(content), 0o755); err != nil {
		return fmt.Errorf("write post-commit hook: %w", err)
	}
	if err := os.Chmod(hookPath, 0o755); err != nil {
		return fmt.Errorf("set hook permissions: %w", err)
	}
	return nil
}

// RemovePostCommitHook removes only the devmem line from post-commit hook.
func RemovePostCommitHook(repoRoot string) error {
	hookPath := filepath.Join(repoRoot, ".git", "hooks", "post-commit")
	b, err := os.ReadFile(hookPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read post-commit hook: %w", err)
	}
	lines := strings.Split(string(b), "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.Contains(line, hookLine) {
			continue
		}
		filtered = append(filtered, line)
	}
	content := strings.Join(filtered, "\n")
	if err := os.WriteFile(hookPath, []byte(content), 0o755); err != nil {
		return fmt.Errorf("write post-commit hook: %w", err)
	}
	return nil
}
