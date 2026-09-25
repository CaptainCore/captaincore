package models

import "testing"

func TestIsUnmappedTenant(t *testing.T) {
	cases := []struct {
		name    string
		home    string
		details string
		want    bool
	}{
		{"unmapped tenant", "https://host.example", `{"freighter":{"role":"tenant","tenant_id":3,"main_url":"https://host.example/"}}`, true},
		{"mapped tenant", "https://tenant.example", `{"freighter":{"role":"tenant","tenant_id":3,"main_url":"https://host.example"}}`, false},
		{"host", "https://host.example", `{"freighter":{"role":"host","main_url":"https://host.example"}}`, false},
		{"plain site", "https://site.example", `{"freighter":null}`, false},
		{"no details", "https://site.example", ``, false},
	}
	for _, c := range cases {
		e := Environment{HomeURL: c.home, Details: c.details}
		if got := e.IsUnmappedTenant(); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
