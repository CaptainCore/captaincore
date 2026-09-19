package cmd

import "testing"

func TestSecurityPluginChanges(t *testing.T) {
	prev := `[{"name":"wp-2fa","status":"active"},{"name":"wordfence","status":"active"},{"name":"akismet","status":"active"},{"name":"loginizer","status":"inactive"}]`
	cur := `[{"name":"wp-2fa","status":"inactive"},{"name":"akismet","status":"inactive"},{"name":"loginizer","status":"inactive"}]`
	got := securityPluginChanges(prev, cur)
	ids := map[string]string{}
	for _, f := range got {
		ids[f.Filename] = f.SignatureID
	}
	if ids["plugin:wp-2fa"] != "security-plugin-deactivated" || ids["plugin:wordfence"] != "security-plugin-removed" || len(ids) != 2 {
		t.Errorf("got %v", ids)
	}
	if securityPluginChanges("", cur) != nil || securityPluginChanges(prev, "[]") != nil {
		t.Error("a missing list must not produce findings")
	}
}
