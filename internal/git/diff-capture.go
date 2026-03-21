package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// GetCurrentCommit returns HEAD commit hash.
func GetCurrentCommit(repoRoot string) (string, error) {
	out, err := runGit(repoRoot, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// GetChangedFiles returns changed file paths between commits.
func GetChangedFiles(repoRoot, fromCommit, toCommit string) ([]string, error) {
	out, err := runGit(repoRoot, "diff", "--name-only", fromCommit, toCommit)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return []string{}, nil
	}
	lines := strings.Split(trimmed, "\n")
	files := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// GetDiffPatch returns full diff patch text between commits.
func GetDiffPatch(repoRoot, fromCommit, toCommit string) (string, error) {
	out, err := runGit(repoRoot, "diff", fromCommit, toCommit)
	if err != nil {
		return "", err
	}
	if len(out) > 4000 {
		return out[:4000], nil
	}
	return out, nil
}

func runGit(repoRoot string, args ...string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git is not installed or not available in PATH")
	}
	cmdArgs := append([]string{"-C", repoRoot}, args...)
	cmd := exec.Command("git", cmdArgs...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText == "" {
			errText = err.Error()
		}
		if strings.Contains(strings.ToLower(errText), "not a git repository") {
			return "", fmt.Errorf("directory is not a git repository: %s", repoRoot)
		}
		return "", fmt.Errorf("git %s failed: %s", strings.Join(args, " "), errText)
	}
	return stdout.String(), nil
}
