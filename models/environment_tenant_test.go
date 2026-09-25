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

func TestDatabaseDumpNames(t *testing.T) {
	tenant := &Site{Details: `{"environment_vars":[{"key":"STACKED_SITE_ID","value":"7"}]}`}
	if got := tenant.DatabaseDumpNames(); len(got) != 2 || got[0] != "database-backup-7.sql" || got[1] != "database-backup.sql" {
		t.Errorf("tenant: got %v", got)
	}
	plain := &Site{Details: `{"environment_vars":""}`}
	if got := plain.DatabaseDumpNames(); len(got) != 1 || got[0] != "database-backup.sql" {
		t.Errorf("plain: got %v", got)
	}
	if got := (*Site)(nil).TenantID(); got != "" {
		t.Errorf("nil site: got %q", got)
	}
}
