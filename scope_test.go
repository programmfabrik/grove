package main

import "testing"

// plainSha is what keeps a scope id from reaching git as anything but a hash.
func TestPlainSha(t *testing.T) {
	for _, s := range []string{"81b535fb", "0123456789abcdef0123456789abcdef01234567"} {
		if !plainSha(s) {
			t.Errorf("plainSha(%q) = false, want true", s)
		}
	}
	for _, s := range []string{
		"",
		"81b535",                   // too short to be a prefix worth resolving
		"81b535fb^",                // a revision expression, not a hash
		"HEAD",                     //
		"81b535fb --upload-pack=x", //
		"81B535FB",                 // git prints lowercase; anything else is not from us
		"0123456789abcdef0123456789abcdef012345678", // too long
	} {
		if plainSha(s) {
			t.Errorf("plainSha(%q) = true, want false", s)
		}
	}
}
