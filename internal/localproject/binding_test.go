package localproject

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	bindingProjectIDOne    = "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"
	bindingRepositoryIDOne = "018f784a-68c1-7b0f-8f2a-cfc255f99e2e"
	bindingWorkspaceIDOne  = "018f784a-68c1-7b0f-8f2a-cfc255f99e3f"
	bindingProjectIDTwo    = "028f784a-68c1-7b0f-8f2a-cfc255f99e1d"
	bindingRepositoryIDTwo = "028f784a-68c1-7b0f-8f2a-cfc255f99e2e"
	bindingWorkspaceIDTwo  = "028f784a-68c1-7b0f-8f2a-cfc255f99e3f"
)

func TestLegacyBindingLoadsOfflineAndMigratesAtomicallyToV2(t *testing.T) {
	root := newBindingTestRepository(t, "https://oauth2:top-secret@github.com/example/legacy.git")
	result, err := Init(InitOptions{StartPath: root, Name: "Legacy", ServerURL: "https://one.pact.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	legacy := `{"schema_version":1,"server_url":"https://one.pact.example.com","project_id":"` + bindingProjectIDOne + `"}`
	if err := os.WriteFile(result.LocalConfigPath, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	binding, err := LoadBinding(root)
	if err != nil {
		t.Fatal(err)
	}
	if !binding.NeedsMigration || binding.SchemaVersion != 1 || binding.ProjectID != bindingProjectIDOne || binding.GitRemoteFingerprint == "" {
		t.Fatalf("legacy binding = %#v", binding)
	}

	migrated, err := Bind(root, BindOptions{
		ServerURL: "https://one.pact.example.com", WorkspaceID: bindingWorkspaceIDOne,
		RepositoryID: bindingRepositoryIDOne, ProjectID: bindingProjectIDOne,
	})
	if err != nil {
		t.Fatal(err)
	}
	if migrated.SchemaVersion != LocalBindingSchemaVersion || migrated.NeedsMigration ||
		migrated.WorkspaceID != bindingWorkspaceIDOne || migrated.RepositoryID != bindingRepositoryIDOne ||
		migrated.ConfiguredAt.IsZero() {
		t.Fatalf("migrated binding = %#v", migrated)
	}
	content := readFile(t, result.LocalConfigPath)
	if strings.Contains(content, "top-secret") || strings.Contains(content, "github.com/example/legacy") ||
		!strings.Contains(content, `"git_remote_fingerprint": "sha256:`) {
		t.Fatalf("migrated config exposes a remote or lacks its fingerprint: %s", content)
	}
	before := content
	if _, err := Bind(root, BindOptions{
		ServerURL: "https://one.pact.example.com", WorkspaceID: bindingWorkspaceIDOne,
		RepositoryID: bindingRepositoryIDOne, ProjectID: bindingProjectIDOne,
	}); err != nil {
		t.Fatal(err)
	}
	if after := readFile(t, result.LocalConfigPath); after != before {
		t.Fatalf("idempotent Bind changed config:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestRebindRequiresConfirmationAndRotatesNodeIdentity(t *testing.T) {
	root := newBindingTestRepository(t, "https://github.com/example/rebind.git")
	if _, err := Init(InitOptions{StartPath: root, ServerURL: "https://one.pact.example.com"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Bind(root, BindOptions{
		ServerURL: "https://one.pact.example.com", WorkspaceID: bindingWorkspaceIDOne,
		RepositoryID: bindingRepositoryIDOne, ProjectID: bindingProjectIDOne,
	}); err != nil {
		t.Fatal(err)
	}
	firstNode, err := EnsureNodeIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	if firstNode.ServerURL != "https://one.pact.example.com" || firstNode.SchemaVersion != nodeIdentitySchemaVersion {
		t.Fatalf("first node = %#v", firstNode)
	}
	if _, err := Bind(root, BindOptions{
		ServerURL: "https://two.pact.example.com", WorkspaceID: bindingWorkspaceIDTwo,
		RepositoryID: bindingRepositoryIDTwo, ProjectID: bindingProjectIDTwo,
	}); err == nil || !strings.Contains(err.Error(), "--rebind") {
		t.Fatalf("Bind without rebind error = %v", err)
	}
	stillFirst, err := LoadBinding(root)
	if err != nil || stillFirst.ServerURL != "https://one.pact.example.com" {
		t.Fatalf("binding changed without confirmation = %#v, %v", stillFirst, err)
	}

	second, err := Bind(root, BindOptions{
		ServerURL: "https://two.pact.example.com", WorkspaceID: bindingWorkspaceIDTwo,
		RepositoryID: bindingRepositoryIDTwo, ProjectID: bindingProjectIDTwo, Rebind: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.ServerURL != "https://two.pact.example.com" {
		t.Fatalf("second binding = %#v", second)
	}
	secondNode, err := EnsureNodeIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	if secondNode.Key == firstNode.Key || secondNode.ServerURL != second.ServerURL {
		t.Fatalf("node identity did not rotate: before=%#v after=%#v", firstNode, secondNode)
	}
	if _, err := Bind(root, BindOptions{
		ServerURL: second.ServerURL, WorkspaceID: second.WorkspaceID,
		RepositoryID: second.RepositoryID, ProjectID: second.ProjectID, Rebind: true,
	}); err != nil {
		t.Fatal(err)
	}
	stableNode, err := EnsureNodeIdentity(root)
	if err != nil || stableNode.Key != secondNode.Key {
		t.Fatalf("idempotent rebind rotated node = %#v, %v", stableNode, err)
	}
}

func TestLegacyNodeIdentityKeepsItsKeyOnSameServer(t *testing.T) {
	root := newBindingTestRepository(t, "https://github.com/example/legacy-node.git")
	result, err := Init(InitOptions{StartPath: root, ServerURL: "https://one.pact.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	legacyKey := "038f784a-68c1-7b0f-8f2a-cfc255f99e4a"
	legacyNode := `{"schema_version":1,"node_key":"` + legacyKey + `","name":"Legacy node"}`
	if err := os.WriteFile(filepath.Join(result.LocalDirectory, "node.json"), []byte(legacyNode), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Bind(root, BindOptions{
		ServerURL: "https://one.pact.example.com", WorkspaceID: bindingWorkspaceIDOne,
		RepositoryID: bindingRepositoryIDOne, ProjectID: bindingProjectIDOne,
	}); err != nil {
		t.Fatal(err)
	}
	identity, err := EnsureNodeIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Key != legacyKey || identity.SchemaVersion != nodeIdentitySchemaVersion ||
		identity.ServerURL != "https://one.pact.example.com" {
		t.Fatalf("legacy node identity was not upgraded in place: %#v", identity)
	}
}

func TestChangedGitRemoteRequiresExplicitRebind(t *testing.T) {
	root := newBindingTestRepository(t, "https://github.com/example/original.git")
	if _, err := Init(InitOptions{StartPath: root, ServerURL: "https://pact.example.com"}); err != nil {
		t.Fatal(err)
	}
	options := BindOptions{
		ServerURL: "https://pact.example.com", WorkspaceID: bindingWorkspaceIDOne,
		RepositoryID: bindingRepositoryIDOne, ProjectID: bindingProjectIDOne,
	}
	first, err := Bind(root, options)
	if err != nil {
		t.Fatal(err)
	}
	runBindingGit(t, root, "remote", "set-url", "origin", "git@github.com:example/replacement.git")
	if _, err := LoadBinding(root); err == nil || !strings.Contains(err.Error(), "remote origin changed") {
		t.Fatalf("LoadBinding after remote change error = %v", err)
	}
	if _, err := Bind(root, options); err == nil || !strings.Contains(err.Error(), "--rebind") {
		t.Fatalf("Bind after remote change error = %v", err)
	}
	options.Rebind = true
	rebound, err := Bind(root, options)
	if err != nil {
		t.Fatal(err)
	}
	if rebound.GitRemoteFingerprint == first.GitRemoteFingerprint {
		t.Fatal("remote fingerprint did not change after explicit rebind")
	}
}

func TestSeparateWorktreeCanBindAnotherServer(t *testing.T) {
	root := newBindingTestRepository(t, "https://github.com/example/worktrees.git")
	if _, err := Init(InitOptions{StartPath: root, Name: "Worktrees", ServerURL: "https://one.pact.example.com"}); err != nil {
		t.Fatal(err)
	}
	runBindingGit(t, root, "add", "pact.yaml", ".gitignore")
	runBindingGit(t, root, "commit", "-m", "Add Pact manifest")
	worktree := filepath.Join(t.TempDir(), "second-checkout")
	runBindingGit(t, root, "worktree", "add", "-b", "second-server", worktree)
	if _, err := Init(InitOptions{StartPath: worktree, ServerURL: "https://two.pact.example.com"}); err != nil {
		t.Fatal(err)
	}
	first, err := Bind(root, BindOptions{
		ServerURL: "https://one.pact.example.com", WorkspaceID: bindingWorkspaceIDOne,
		RepositoryID: bindingRepositoryIDOne, ProjectID: bindingProjectIDOne,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Bind(worktree, BindOptions{
		ServerURL: "https://two.pact.example.com", WorkspaceID: bindingWorkspaceIDTwo,
		RepositoryID: bindingRepositoryIDTwo, ProjectID: bindingProjectIDTwo,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Root == second.Root || first.ServerURL == second.ServerURL {
		t.Fatalf("worktree bindings are not independent: first=%#v second=%#v", first, second)
	}
}

func TestFailedAtomicRebindKeepsPreviousBinding(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory write permissions differ on Windows")
	}
	root := newBindingTestRepository(t, "https://github.com/example/atomic.git")
	result, err := Init(InitOptions{StartPath: root, ServerURL: "https://one.pact.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Bind(root, BindOptions{
		ServerURL: "https://one.pact.example.com", WorkspaceID: bindingWorkspaceIDOne,
		RepositoryID: bindingRepositoryIDOne, ProjectID: bindingProjectIDOne,
	}); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, result.LocalConfigPath)
	localPath := filepath.Dir(result.LocalConfigPath)
	if err := os.Chmod(localPath, 0o500); err != nil {
		t.Fatal(err)
	}
	_, bindErr := Bind(root, BindOptions{
		ServerURL: "https://two.pact.example.com", WorkspaceID: bindingWorkspaceIDTwo,
		RepositoryID: bindingRepositoryIDTwo, ProjectID: bindingProjectIDTwo, Rebind: true,
	})
	if err := os.Chmod(localPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if bindErr == nil {
		t.Fatal("Bind succeeded while the local directory was read-only")
	}
	if after := readFile(t, result.LocalConfigPath); after != before {
		t.Fatalf("failed atomic write changed binding:\nbefore=%s\nafter=%s", before, after)
	}
}

func newBindingTestRepository(t *testing.T, remote string) string {
	t.Helper()
	root := t.TempDir()
	runBindingGit(t, root, "init", "-b", "main")
	runBindingGit(t, root, "config", "user.email", "pact@example.com")
	runBindingGit(t, root, "config", "user.name", "Pact Test")
	runBindingGit(t, root, "remote", "add", "origin", remote)
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Binding test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runBindingGit(t, root, "add", "README.md")
	runBindingGit(t, root, "commit", "-m", "Initial commit")
	return root
}

func runBindingGit(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
}
