package cmd

import (
	"encoding/json"
	"testing"
)

func TestConfigValueString(t *testing.T) {
	cases := map[string]string{
		`"1"`:      "1",
		`1`:        "1",
		`42`:       "42",
		`"id_rsa"`: "id_rsa",
		`""`:       "",
		`null`:     "",
		`true`:     "",
		`{}`:       "",
	}
	for raw, want := range cases {
		if got := configValueString(json.RawMessage(raw)); got != want {
			t.Errorf("configValueString(%s) = %q, want %q", raw, got, want)
		}
	}
}

func TestParseSiteArgumentDomain(t *testing.T) {
	cases := map[string]SiteArg{
		"mysite":                     {SiteName: "mysite", Environment: "production"},
		"mysite-staging":             {SiteName: "mysite", Environment: "staging"},
		"mysite@kinsta":              {SiteName: "mysite", Environment: "production", Provider: "kinsta"},
		"blog.example.com":           {SiteName: "blog.example.com", Environment: "production", Domain: true},
		"blog.example.com-staging":   {SiteName: "blog.example.com", Environment: "staging", Domain: true},
		"my-site.com":                {SiteName: "my-site.com", Environment: "production", Domain: true},
		"my-site.com-staging":        {SiteName: "my-site.com", Environment: "staging", Domain: true},
		"my-site.com@kinsta":         {SiteName: "my-site.com", Environment: "production", Provider: "kinsta", Domain: true},
		"my-site.com-staging@kinsta": {SiteName: "my-site.com", Environment: "staging", Provider: "kinsta", Domain: true},
		"my-site.com@kinsta-staging": {SiteName: "my-site.com", Environment: "staging", Provider: "kinsta", Domain: true},
	}
	for arg, want := range cases {
		if got := parseSiteArgument(arg); got != want {
			t.Errorf("parseSiteArgument(%q) = %+v, want %+v", arg, got, want)
		}
	}
}
