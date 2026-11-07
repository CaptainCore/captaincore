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

// breakwaterWordsMaxAge is how long the word model fetched from the Manager is shared by syncs.
// The Manager rebuilds it at most twice a day.
const breakwaterWordsMaxAge = 12 * time.Hour

// breakwaterWordsRefresh is how old a site's word model may get before a sync sends a newer one.
// Breakwater ignores a model older than 7 days; the model is ~600 KB, so not every day.
const breakwaterWordsRefresh = 3 * 24 * time.Hour

// breakwaterWords returns the word model (JSON) and its generated stamp, from the cache in
// ~/.captaincore/data when it is fresh, from the Manager's spam-words command otherwise. Nil while
// the model is empty (the fleet hasn't counted both spam and kept messages yet).
func breakwaterWords(client *apiclient.Client) ([]byte, string) {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".captaincore", "data", "breakwater-words.json")
	if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) < breakwaterWordsMaxAge {
		if body, err := os.ReadFile(path); err == nil {
			if generated, clues := breakwaterWordsMeta(body); generated != "" && clues > 0 {
				return body, generated
			}
		}
	}
	body, err := client.Post("spam-words", map[string]interface{}{})
	generated, clues := breakwaterWordsMeta(body)
	if err != nil || generated == "" || clues == 0 {
		return nil, ""
	}
	_ = os.WriteFile(path, body, 0600)
	return body, generated
}

func breakwaterWordsMeta(body []byte) (string, int) {
	var model struct {
		V         int            `json:"v"`
		Generated string         `json:"generated"`
		P         map[string]int `json:"p"`
	}
	if json.Unmarshal(body, &model) != nil || model.V != 1 {
		return "", 0
	}
	return model.Generated, len(model.P)
}

// breakwaterWordsDue says whether a site holding a model made at `held` should get the one made
// at `generated`: when it has none, or its copy is older than breakwaterWordsRefresh.
func breakwaterWordsDue(held, generated string, now time.Time) bool {
	if generated == "" || held == generated {
		return false
	}
	at, err := time.Parse(time.RFC3339, held)
	return err != nil || now.Sub(at) > breakwaterWordsRefresh
}

// pushBreakwaterWords sends the word model to a site whose spam signal says its Breakwater scores
// words and holds no model, or one older than three days. Best-effort, like the fleet list.
func pushBreakwaterWords(site string, signal string, client *apiclient.Client, quiet bool) {
	var state struct {
		Breakwater struct {
			Words   string `json:"words"`
			WordsOK bool   `json:"words_ok"`
		} `json:"breakwater"`
	}
	if json.Unmarshal([]byte(signal), &state) != nil || !state.Breakwater.WordsOK {
		return
	}
	body, generated := breakwaterWords(client)
	if body == nil || !breakwaterWordsDue(state.Breakwater.Words, generated, time.Now()) {
		return
	}
	script, err := os.CreateTemp("", "breakwater-words-*.sh")
	if err != nil {
		return
	}
	defer os.Remove(script.Name())
	script.WriteString(breakwaterWordsScript(body))
	script.Close()
	out, _ := exec.Command("captaincore", "ssh", site, "--script="+script.Name(), "--captain-id="+captainID).Output()
	if !quiet {
		fmt.Printf("Breakwater word model (%s): %s\n", generated, trimOutput(string(out)))
	}
}

// breakwaterWordsScript is the remote script that stores the model: it rides in a quoted heredoc
// (hex keys and integers only, never a quote that ends it or a dollar), and Breakwater validates it.
func breakwaterWordsScript(body []byte) string {
	return "#!/bin/bash\nread -r -d '' model <<'BREAKWATERWORDSEOF'\n" + string(body) + "\nBREAKWATERWORDSEOF\n" +
		"printf '%s' \"$model\" | wp eval 'echo class_exists(\"Breakwater\\\\Words\") && Breakwater\\Words::store( stream_get_contents( STDIN ) ) ? \"breakwater-words:stored\" : \"breakwater-words:rejected\";' --skip-themes 2>/dev/null\n"
}

func trimOutput(s string) string {
	if len(s) > 80 {
		return s[:80]
	}
	return s
}
