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
