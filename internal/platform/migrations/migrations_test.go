package migrations

import (
	"strings"
	"testing"
	"time"
)

func TestValidateHistory(t *testing.T) {
	definitions := []definition{
		{Version: "000001_core", Checksum: "first"},
		{Version: "000002_next", Checksum: "second"},
	}
	applied := []Applied{
		{Version: "000001_core", Checksum: "first", AppliedAt: time.Now()},
	}

	tests := []struct {
		name        string
		definitions []definition
		applied     []Applied
		exact       bool
		wantError   string
	}{
		{
			name:        "prefix accepted while migrating",
			definitions: definitions,
			applied:     applied,
		},
		{
			name:        "exact history accepted",
			definitions: definitions[:1],
			applied:     applied,
			exact:       true,
		},
		{
			name:        "pending rejected while verifying",
			definitions: definitions,
			applied:     applied,
			exact:       true,
			wantError:   "required migrations",
		},
		{
			name:        "newer database rejected",
			definitions: definitions[:1],
			applied: append(applied, Applied{
				Version:  "000002_next",
				Checksum: "second",
			}),
			wantError: "older binary",
		},
		{
			name:        "non-prefix history rejected",
			definitions: definitions,
			applied: []Applied{
				{Version: "000002_next", Checksum: "second"},
			},
			wantError: "not a prefix",
		},
		{
			name:        "changed checksum rejected",
			definitions: definitions,
			applied: []Applied{
				{Version: "000001_core", Checksum: "changed"},
			},
			wantError: "checksum",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateHistory(test.definitions, test.applied, test.exact)
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("validateHistory() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("validateHistory() error = %v, want containing %q", err, test.wantError)
			}
		})
	}
}
