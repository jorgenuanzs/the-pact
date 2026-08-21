package gitremote

import "testing"

func TestNormalizeTreatsGitHubTransportsAsTheSameRepository(t *testing.T) {
	inputs := []string{
		"https://github.com/example/repository.git",
		"git@github.com:example/repository.git",
		"ssh://git@github.com/example/repository.git",
		"git://github.com/example/repository.git",
	}
	for _, input := range inputs {
		value, err := Normalize(input)
		if err != nil {
			t.Fatalf("Normalize(%q): %v", input, err)
		}
		if value != "https://github.com/example/repository" {
			t.Fatalf("Normalize(%q) = %q", input, value)
		}
	}
}

func TestNormalizeRemovesCredentialsAndRejectsLocalPaths(t *testing.T) {
	value, err := Normalize("https://oauth:secret@github.com/example/private.git?token=secret#fragment")
	if err != nil {
		t.Fatal(err)
	}
	if value != "https://github.com/example/private" {
		t.Fatalf("normalized credential remote = %q", value)
	}
	if _, err := Normalize("../private"); err == nil {
		t.Fatal("Normalize accepted a local path")
	}
}
