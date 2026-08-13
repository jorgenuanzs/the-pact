package gitobserve

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type Snapshot struct {
	Dirty        bool
	Fingerprint  string
	ChangedPaths int
	HeadRevision string
	Branch       string
}

func Capture(ctx context.Context, root string) (Snapshot, error) {
	status, err := gitOutput(ctx, root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return Snapshot{}, fmt.Errorf("inspect Git worktree: %w", err)
	}
	paths, changedPaths := changedPathList(status)
	fingerprint := sha256.New()
	_, _ = fingerprint.Write([]byte("pact.repository-observation.v1\x00"))
	_, _ = fingerprint.Write(status)
	sort.Strings(paths)
	for _, path := range paths {
		writeFileState(fingerprint, root, path)
	}

	headRevision, err := optionalGitOutput(ctx, root, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return Snapshot{}, fmt.Errorf("read Git HEAD: %w", err)
	}
	branch, err := optionalGitOutput(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return Snapshot{}, fmt.Errorf("read Git branch: %w", err)
	}
	return Snapshot{
		Dirty:        changedPaths > 0,
		Fingerprint:  hex.EncodeToString(fingerprint.Sum(nil)),
		ChangedPaths: changedPaths,
		HeadRevision: strings.TrimSpace(headRevision),
		Branch:       strings.TrimSpace(branch),
	}, nil
}

func changedPathList(status []byte) ([]string, int) {
	records := strings.Split(string(status), "\x00")
	paths := make([]string, 0, len(records))
	changedPaths := 0
	for index := 0; index < len(records); index++ {
		record := records[index]
		if len(record) < 4 {
			continue
		}
		code := record[:2]
		paths = append(paths, record[3:])
		changedPaths++
		if strings.ContainsAny(code, "RC") && index+1 < len(records) {
			index++
		}
	}
	return paths, changedPaths
}

func writeFileState(target hash.Hash, root string, relativePath string) {
	_, _ = target.Write([]byte{0})
	_, _ = target.Write([]byte(relativePath))
	cleaned := filepath.Clean(relativePath)
	if filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		_, _ = target.Write([]byte("\x00outside-root"))
		return
	}
	info, err := os.Lstat(filepath.Join(root, cleaned))
	if err != nil {
		_, _ = target.Write([]byte("\x00missing"))
		return
	}
	var numeric [24]byte
	binary.LittleEndian.PutUint64(numeric[0:8], uint64(info.Mode()))
	binary.LittleEndian.PutUint64(numeric[8:16], uint64(info.Size()))
	binary.LittleEndian.PutUint64(numeric[16:24], uint64(info.ModTime().UnixNano()))
	_, _ = target.Write(numeric[:])
	if info.Mode()&os.ModeSymlink != 0 {
		if destination, readErr := os.Readlink(filepath.Join(root, cleaned)); readErr == nil {
			_, _ = target.Write([]byte(destination))
		}
	}
}

func gitOutput(ctx context.Context, root string, arguments ...string) ([]byte, error) {
	commandArguments := append([]string{"-C", root}, arguments...)
	command := exec.CommandContext(ctx, "git", commandArguments...)
	output, err := command.Output()
	if err == nil {
		return output, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		detail := strings.TrimSpace(string(exitError.Stderr))
		if detail != "" {
			return nil, fmt.Errorf("git %s: %s", strings.Join(arguments, " "), detail)
		}
	}
	return nil, fmt.Errorf("git %s: %w", strings.Join(arguments, " "), err)
}

func optionalGitOutput(ctx context.Context, root string, arguments ...string) (string, error) {
	commandArguments := append([]string{"-C", root}, arguments...)
	output, err := exec.CommandContext(ctx, "git", commandArguments...).Output()
	if err == nil {
		return string(output), nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return "", nil
	}
	return "", err
}
