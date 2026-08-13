//go:build !windows

package localproject

func ensurePlatformGitConfig(string) error {
	return nil
}
