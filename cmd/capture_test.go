package cmd

import "testing"

func TestSameOriginAcrossSiteEnvironments(t *testing.T) {
	origins := "https://staging-example.kinsta.cloud https://www.example.com"
	cases := map[string]bool{
		"staging-example.kinsta.cloud": true,
		"www.example.com":              true,
		"example.com":                  false, // the site's own bare apex is not a suffix of www.example.com
		"cdn.example.com":              false, // subdomains of the home host count, siblings do not (existing behaviour)
		"evil-example.com":             false,
		"kinsta.cloud":                 false,
		"":                             false,
	}
	for domain, want := range cases {
		if got := isSameOrigin(domain, origins); got != want {
			t.Errorf("isSameOrigin(%q) = %v, want %v", domain, got, want)
		}
	}
	if isSameOrigin("www.example.com", "") {
		t.Error("no home URL must never be same-origin")
	}
	if !isSameOrigin("www.example.com", "https://www.example.com") {
		t.Error("single home URL still works")
	}
}
