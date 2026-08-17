package authn

import (
	"errors"
	"strings"
	"testing"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	const password = "a long and memorable passphrase"
	hash, err := hashPassword(password)
	if err != nil {
		t.Fatalf("hashPassword() error = %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("hash = %q", hash)
	}
	if !verifyPassword(password, hash) {
		t.Fatal("correct password was rejected")
	}
	if verifyPassword("a different and incorrect password", hash) {
		t.Fatal("incorrect password was accepted")
	}
	if passwordHashNeedsUpgrade(hash) {
		t.Fatal("new password hash unexpectedly needs an upgrade")
	}
}

func TestPasswordVerificationRejectsMalformedOrExpensiveHashes(t *testing.T) {
	tests := []string{
		"",
		"$argon2id$v=19$m=999999999,t=2,p=1$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA",
		"$argon2id$v=19$m=19456,t=0,p=1$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA",
		"$argon2i$v=19$m=19456,t=2,p=1$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA",
	}
	for _, encoded := range tests {
		if verifyPassword("irrelevant password", encoded) {
			t.Fatalf("malformed hash was accepted: %q", encoded)
		}
	}
}

func TestPasswordPolicyUsesLengthWithoutCompositionRules(t *testing.T) {
	if err := validatePassword("correct horse battery staple"); err != nil {
		t.Fatalf("valid passphrase error = %v", err)
	}
	if err := validatePassword("alllowercasebutlong"); err != nil {
		t.Fatalf("valid lowercase password error = %v", err)
	}
	for _, password := range []string{"too short", strings.Repeat("x", 129), strings.Repeat(" ", 20)} {
		var validationErr *ValidationError
		if err := validatePassword(password); !errors.As(err, &validationErr) {
			t.Fatalf("validatePassword(%q) error = %v", password, err)
		}
	}
}
