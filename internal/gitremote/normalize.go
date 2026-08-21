package gitremote

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var scpPattern = regexp.MustCompile(`^[^/@:]+@([^/:]+):(.+)$`)

// Normalize returns a credential-free, transport-independent identity for a
// Git remote. It is shared by PACT Server, Desktop and the CLI so a checkout
// using SSH resolves to the same repository registered through HTTPS.
func Normalize(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if matches := scpPattern.FindStringSubmatch(raw); len(matches) == 3 {
		raw = "https://" + matches[1] + "/" + matches[2]
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse Git remote origin: %w", err)
	}
	if parsed.Host == "" {
		return "", errors.New("Git remote origin must be a network URL")
	}
	switch parsed.Scheme {
	case "https", "http", "ssh", "git":
	default:
		return "", fmt.Errorf("unsupported Git remote scheme %q", parsed.Scheme)
	}
	if parsed.Scheme == "ssh" || parsed.Scheme == "git" {
		parsed.Scheme = "https"
	}
	parsed.User = nil
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimSuffix(strings.TrimRight(parsed.Path, "/"), ".git")
	if parsed.Path == "" || parsed.Path == "/" {
		return "", errors.New("Git remote origin must identify a repository")
	}
	return parsed.String(), nil
}
