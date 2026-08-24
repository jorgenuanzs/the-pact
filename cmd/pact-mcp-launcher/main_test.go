package main

import "testing"

func TestValidDigest(t *testing.T) {
	for _, test := range []struct {
		Value string
		Valid bool
	}{
		{Value: "0123456789ab", Valid: true},
		{Value: "0123456789AB", Valid: false},
		{Value: "../../runtime", Valid: false},
		{Value: "0123456789a", Valid: false},
	} {
		if validDigest(test.Value) != test.Valid {
			t.Fatalf("validDigest(%q) = %t, want %t", test.Value, !test.Valid, test.Valid)
		}
	}
}
