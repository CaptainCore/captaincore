package cmd

import "testing"

func TestOriginFromURL(t *testing.T) {
	cases := map[string]string{
		"https://anchor.host":            "https://anchor.host",
		"https://Anchor.Host/account/":   "https://anchor.host",
		"http://127.0.0.1:8000/wp-admin": "http://127.0.0.1:8000",
		"https://manager.localhost":      "https://manager.localhost",
		"":                               "",
		"not a url":                      "",
		"anchor.host":                    "",
	}
	for in, want := range cases {
		if got := originFromURL(in); got != want {
			t.Errorf("originFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}
