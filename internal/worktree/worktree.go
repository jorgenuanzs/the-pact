package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

type Result struct {
	Path         string
	PathRef      string
	Branch       string
	BaseRevision string
}

func Create(ctx context.Context, projectRoot, intentID, title, baseRevision string) (Result, error) {
	projectRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return Result{}, fmt.Errorf("resolve project root: %w", err)
	}
	baseRevision = strings.ToLower(strings.TrimSpace(baseRevision))
	resolvedBase, err := gitOutput(ctx, projectRoot, "rev-parse", "--verify", baseRevision+"^{commit}")
	if err != nil {
		return Result{}, fmt.Errorf("resolve worktree base revision: %w", err)
	}
	branch := "pact/" + shortID(intentID) + "-" + slug(title)
	pathRef := filepath.ToSlash(filepath.Join(".pact", "worktrees", intentID))
	worktreePath := filepath.Join(projectRoot, filepath.FromSlash(pathRef))

	if info, statErr := os.Lstat(worktreePath); statErr == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return Result{}, errors.New("the managed worktree path exists but is not a real directory")
		}
		currentBranch, branchErr := gitOutput(ctx, worktreePath, "branch", "--show-current")
		_, baseErr := gitOutput(ctx, worktreePath, "merge-base", "--is-ancestor", resolvedBase, "HEAD")
		if branchErr == nil && baseErr == nil && currentBranch == branch {
			return Result{Path: worktreePath, PathRef: pathRef, Branch: branch, BaseRevision: resolvedBase}, nil
		}
		return Result{}, errors.New("the managed worktree path already belongs to different work")
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return Result{}, fmt.Errorf("inspect managed worktree path: %w", statErr)
	}

	parent := filepath.Dir(worktreePath)
	if err := ensurePrivateRealDirectory(parent); err != nil {
		return Result{}, err
	}
	addArguments := []string{"worktree", "add", "-b", branch, worktreePath, resolvedBase}
	if branchRevision, branchErr := gitOutput(ctx, projectRoot, "rev-parse", "--verify", "refs/heads/"+branch+"^{commit}"); branchErr == nil {
		if _, ancestorErr := gitOutput(ctx, projectRoot, "merge-base", "--is-ancestor", resolvedBase, branchRevision); ancestorErr != nil {
			return Result{}, errors.New("the managed branch exists but does not descend from the requested base revision")
		}
		addArguments = []string{"worktree", "add", worktreePath, branch}
	}
	if _, err := gitOutput(ctx, projectRoot, addArguments...); err != nil {
		return Result{}, fmt.Errorf("create managed Git worktree: %w", err)
	}
	created := true
	defer func() {
		if created {
			_, _ = gitOutput(context.Background(), projectRoot, "worktree", "remove", "--force", worktreePath)
		}
	}()
	if err := copyLocalRuntime(projectRoot, worktreePath); err != nil {
		return Result{}, err
	}
	created = false
	return Result{Path: worktreePath, PathRef: pathRef, Branch: branch, BaseRevision: resolvedBase}, nil
}

func ensurePrivateRealDirectory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("create managed worktree directory: %w", err)
		}
		return os.Chmod(path, 0o700)
	}
	if err != nil {
		return fmt.Errorf("inspect managed worktree directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("managed worktree parent must be a real directory")
	}
	return os.Chmod(path, 0o700)
}

func copyLocalRuntime(projectRoot, worktreePath string) error {
	destination := filepath.Join(worktreePath, ".pact")
	if err := os.Mkdir(destination, 0o700); err != nil {
		return fmt.Errorf("create worktree Pact runtime: %w", err)
	}
	for _, name := range []string{"config.json", "node.json"} {
		source := filepath.Join(projectRoot, ".pact", name)
		content, err := os.ReadFile(source)
		if errors.Is(err, os.ErrNotExist) && name == "node.json" {
			continue
		}
		if err != nil {
			return fmt.Errorf("read local Pact %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(destination, name), content, 0o600); err != nil {
			return fmt.Errorf("write worktree Pact %s: %w", name, err)
		}
	}
	return nil
}

func gitOutput(ctx context.Context, directory string, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", directory}, arguments...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return "", errors.New(detail)
	}
	return strings.TrimSpace(string(output)), nil
}

func shortID(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "-", "")
	if len(value) > 12 {
		return value[:12]
	}
	if value == "" {
		return "work"
	}
	return value
}

func slug(value string) string {
	var builder strings.Builder
	previousDash := false
	for _, character := range strings.ToLower(value) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			builder.WriteRune(character)
			previousDash = false
			continue
		}
		if (unicode.IsSpace(character) || character == '-' || character == '_') && builder.Len() > 0 && !previousDash {
			builder.WriteByte('-')
			previousDash = true
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		result = "work"
	}
	if len(result) > 48 {
		result = strings.TrimRight(result[:48], "-")
	}
	return result
}
