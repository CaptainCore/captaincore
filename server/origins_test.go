package server

import "testing"

func TestOriginAllowed(t *testing.T) {
	cfg := []string{"https://anchor.host"}
	env := "https://portal.example.com, https://Other.Example/"
	cases := []struct {
		origin string
		want   bool
	}{
		{"https://anchor.host", true},
		{"https://ANCHOR.host/", true},
		{"https://portal.example.com", true},
		{"https://other.example", true},
		{"http://anchor.host", false},
		{"https://evil.example", false},
		{"", false},
	}
	for _, c := range cases {
		if got := originAllowed(c.origin, cfg, env); got != c.want {
			t.Errorf("originAllowed(%q) = %v, want %v", c.origin, got, c.want)
		}
	}
	if originAllowed("https://anchor.host", nil, "") {
		t.Error("nothing configured must allow nothing")
	}
}
