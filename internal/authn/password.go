package authn

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemory      = 19 * 1024
	argonIterations  = 2
	argonParallelism = 1
	argonSaltLength  = 16
	argonKeyLength   = 32
)

func hashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	return encodePassword(password, salt), nil
}

func encodePassword(password string, salt []byte) string {
	hash := argon2.IDKey(
		[]byte(password), salt, argonIterations, argonMemory,
		argonParallelism, argonKeyLength,
	)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonIterations, argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
}

func verifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	version, err := strconv.Atoi(strings.TrimPrefix(parts[2], "v="))
	if err != nil || version != argon2.Version {
		return false
	}
	var memory uint64
	var iterations uint64
	var parallelism uint64
	parameters := strings.Split(parts[3], ",")
	if len(parameters) != 3 {
		return false
	}
	for _, parameter := range parameters {
		key, value, found := strings.Cut(parameter, "=")
		if !found {
			return false
		}
		parsed, parseErr := strconv.ParseUint(value, 10, 32)
		if parseErr != nil {
			return false
		}
		switch key {
		case "m":
			memory = parsed
		case "t":
			iterations = parsed
		case "p":
			parallelism = parsed
		default:
			return false
		}
	}
	if memory < 8*1024 || memory > 256*1024 || iterations < 1 || iterations > 10 || parallelism < 1 || parallelism > 16 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 || len(salt) > 64 {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) < 16 || len(expected) > 64 {
		return false
	}
	actual := argon2.IDKey(
		[]byte(password), salt, uint32(iterations), uint32(memory),
		uint8(parallelism), uint32(len(expected)),
	)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func dummyPasswordHash() string {
	return encodePassword("pact-dummy-password-never-valid", []byte("pact-dummy-salt!"))
}

func validatePassword(password string) error {
	length := len([]rune(password))
	if length < 15 || length > 128 {
		return &ValidationError{Field: "password", Message: "must contain 15 to 128 characters"}
	}
	if strings.TrimSpace(password) == "" {
		return &ValidationError{Field: "password", Message: "must not contain only whitespace"}
	}
	return nil
}

func passwordHashNeedsUpgrade(encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return true
	}
	expected := fmt.Sprintf("m=%d,t=%d,p=%d", argonMemory, argonIterations, argonParallelism)
	return parts[3] != expected
}
