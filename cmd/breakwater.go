package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/CaptainCore/captaincore/apiclient"
)

// breakwaterListMaxAge is how long the fleet spam list fetched from the Manager is shared by
// syncs before it is fetched again. A sweep of the fleet fetches it once.
const breakwaterListMaxAge = time.Hour

// breakwaterList returns the fleet spam list (JSON) and its generated stamp, from the cache in
// ~/.captaincore/data when it is fresh, from the Manager's spam-reputation command otherwise.
func breakwaterList(client *apiclient.Client) ([]byte, string) {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".captaincore", "data", "breakwater-reputation.json")
	if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) < breakwaterListMaxAge {
		if body, err := os.ReadFile(path); err == nil {
			if generated := breakwaterGenerated(body); generated != "" {
				return body, generated
			}
		}
	}
	body, err := client.Post("spam-reputation", map[string]interface{}{})
	generated := breakwaterGenerated(body)
	if err != nil || generated == "" {
		return nil, ""
	}
	_ = os.WriteFile(path, body, 0600)
	return body, generated
}

func breakwaterGenerated(body []byte) string {
	var list struct {
		Generated string `json:"generated"`
	}
	if json.Unmarshal(body, &list) != nil {
		return ""
	}
	return list.Generated
}

// pushBreakwaterList sends the fleet spam list to a site whose spam signal says Breakwater is
// active and holds a different list. Best-effort: a failure leaves the site on its local checks.
func pushBreakwaterList(site string, signal string, client *apiclient.Client, quiet bool) {
	var state struct {
		Breakwater struct {
			Active bool   `json:"active"`
			List   string `json:"list"`
		} `json:"breakwater"`
	}
	if json.Unmarshal([]byte(signal), &state) != nil || !state.Breakwater.Active {
		return
	}
	body, generated := breakwaterList(client)
	if body == nil || generated == state.Breakwater.List {
		return
	}
	script, err := os.CreateTemp("", "breakwater-list-*.sh")
	if err != nil {
		return
	}
	defer os.Remove(script.Name())
	script.WriteString(breakwaterScript(body))
	script.Close()
	out, _ := exec.Command("captaincore", "ssh", site, "--script="+script.Name(), "--captain-id="+captainID).Output()
	if !quiet {
		fmt.Printf("Breakwater fleet list (%s): %s\n", generated, trimOutput(string(out)))
	}
}

// breakwaterScript is the remote script that stores a list: it rides in a quoted heredoc
// (hashes and numbers only, never a quote or a dollar), and Breakwater validates it before storing.
func breakwaterScript(body []byte) string {
	return "#!/bin/bash\nread -r -d '' list <<'BREAKWATERLISTEOF'\n" + string(body) + "\nBREAKWATERLISTEOF\n" +
		"printf '%s' \"$list\" | wp eval 'echo class_exists(\"Breakwater\\\\Reputation\") && Breakwater\\Reputation::store( (array) json_decode( stream_get_contents( STDIN ), true ) ) ? \"breakwater-list:stored\" : \"breakwater-list:rejected\";' --skip-themes 2>/dev/null\n"
}

func trimOutput(s string) string {
	if len(s) > 80 {
		return s[:80]
	}
	return s
}
