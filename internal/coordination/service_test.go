package coordination

import "testing"

func TestNormalizeScopesCanonicalizesAndDeduplicates(t *testing.T) {
	scopes, err := normalizeScopes([]ScopeInput{
		{Kind: " PATH ", Locator: "internal\\api/../api", Mode: ""},
		{Kind: "path", Locator: "internal/api", Mode: "exclusive"},
		{Kind: "repository", Locator: "ignored", Mode: "shared"},
	})
	if err != nil {
		t.Fatalf("normalizeScopes() error = %v", err)
	}
	if len(scopes) != 2 {
		t.Fatalf("len(scopes) = %d, want 2: %#v", len(scopes), scopes)
	}
	if scopes[0] != (ScopeInput{Kind: "path", Locator: "internal/api", Mode: ClaimModeExclusive}) {
		t.Fatalf("scopes[0] = %#v", scopes[0])
	}
	if scopes[1] != (ScopeInput{Kind: "repository", Locator: ".", Mode: ClaimModeShared}) {
		t.Fatalf("scopes[1] = %#v", scopes[1])
	}
}

func TestNormalizeScopesRejectsPathsOutsideRepository(t *testing.T) {
	for _, locator := range []string{"/etc/passwd", "..", "../secret", "a/../../secret", "."} {
		t.Run(locator, func(t *testing.T) {
			_, err := normalizeScopes([]ScopeInput{{Kind: "path", Locator: locator}})
			if err == nil {
				t.Fatalf("normalizeScopes(%q) unexpectedly succeeded", locator)
			}
			validation, ok := err.(*ValidationError)
			if !ok || validation.Field != "scopes" {
				t.Fatalf("error = %#v, want scopes ValidationError", err)
			}
		})
	}
}

func TestScopesOverlapUnderstandsRepositoryHierarchy(t *testing.T) {
	tests := []struct {
		name        string
		left, right ScopeInput
		want        bool
	}{
		{"repository covers file", ScopeInput{Kind: "repository", Locator: "."}, ScopeInput{Kind: "file", Locator: "README.md"}, true},
		{"parent path covers child file", ScopeInput{Kind: "path", Locator: "internal"}, ScopeInput{Kind: "file", Locator: "internal/api.go"}, true},
		{"child path overlaps parent", ScopeInput{Kind: "path", Locator: "internal/api"}, ScopeInput{Kind: "path", Locator: "internal"}, true},
		{"similar prefix is separate", ScopeInput{Kind: "path", Locator: "internal/api"}, ScopeInput{Kind: "file", Locator: "internal/api2/client.go"}, false},
		{"distinct files are separate", ScopeInput{Kind: "file", Locator: "a.go"}, ScopeInput{Kind: "file", Locator: "b.go"}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := scopesOverlap(test.left, test.right); got != test.want {
				t.Fatalf("scopesOverlap(%#v, %#v) = %v, want %v", test.left, test.right, got, test.want)
			}
		})
	}
}

func TestSharedClaimsDoNotBlockEachOther(t *testing.T) {
	overlaps := []ScopeOverlap{{
		Requested:     ScopeInput{Kind: "path", Locator: "docs", Mode: ClaimModeShared},
		ExistingScope: ScopeInput{Kind: "path", Locator: "docs", Mode: ClaimModeShared},
		Blocking:      false,
	}}
	if hasBlockingOverlap(overlaps) {
		t.Fatal("two shared claims must not block")
	}
	overlaps[0].Requested.Mode = ClaimModeExclusive
	overlaps[0].Blocking = true
	if !hasBlockingOverlap(overlaps) {
		t.Fatal("an exclusive claim must block")
	}
}

func TestIntentStatusTransitions(t *testing.T) {
	for _, transition := range []struct {
		from, to string
		want     bool
	}{
		{"active", "blocked", true},
		{"blocked", "active", true},
		{"active", "submitted", true},
		{"submitted", "completed", true},
		{"completed", "active", false},
		{"active", "completed", false},
		{"active", "active", false},
	} {
		if got := validTransition(transition.from, transition.to); got != transition.want {
			t.Errorf("validTransition(%q, %q) = %v, want %v", transition.from, transition.to, got, transition.want)
		}
	}
}
