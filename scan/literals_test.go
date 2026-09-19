package scan

import (
	"bytes"
	"math/rand"
	"testing"
)

// The index must agree with bytes.Contains for every literal, including
// literals that are prefixes or suffixes of each other and overlapping hits.
func TestLiteralIndexAgreesWithContains(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	alphabet := []byte("ab<?$_e")
	var lits [][]byte
	for i := 0; i < 300; i++ {
		n := 1 + rng.Intn(6)
		b := make([]byte, n)
		for j := range b {
			b[j] = alphabet[rng.Intn(len(alphabet))]
		}
		lits = append(lits, b)
	}
	lits = append(lits, []byte("eval("), []byte("val"), []byte("$_POST"), []byte("_POST"), []byte("aaaa"), []byte("aa"))
	li, ids := newLiteralIndex(lits)
	if li.n != len(ids) {
		t.Fatalf("n=%d ids=%d", li.n, len(ids))
	}
	for round := 0; round < 200; round++ {
		n := rng.Intn(400)
		data := make([]byte, n)
		for j := range data {
			data[j] = alphabet[rng.Intn(len(alphabet))]
		}
		if round%3 == 0 {
			data = append(data, []byte("<?php eval($_POST['x']); aaaa")...)
		}
		hit := make([]bool, li.n)
		li.present(data, hit)
		for lit, id := range ids {
			want := bytes.Contains(data, []byte(lit))
			if hit[id] != want {
				t.Fatalf("literal %q in %q: index says %v, Contains says %v", lit, data, hit[id], want)
			}
		}
	}
}

func TestLiteralIndexDrivesRules(t *testing.T) {
	rs, err := ParseRuleSet([]byte(`[
	  {"id":"a","name":"A","severity":"high","prefilter":["needle"],"patterns":["needle\\s*\\("]},
	  {"id":"b","name":"B","severity":"high","patterns":["\\bexact\\b"]},
	  {"id":"c","name":"C","severity":"high","patterns":["plain literal"]}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	s := New(rs, Options{Workers: 1})
	dir := t.TempDir()
	write(t, dir, "x.php", "<?php needle ( 1 ); inexact plain literal")
	write(t, dir, "y.php", "<?php exact; needle without call")
	res := s.ScanDir(dir)
	got := map[string]bool{}
	for _, f := range res.Findings {
		got[f.File+":"+f.RuleID] = true
	}
	for _, want := range []string{"x.php:a", "x.php:c", "y.php:b"} {
		if !got[want] {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	if got["y.php:a"] || got["x.php:b"] || got["y.php:c"] {
		t.Errorf("unexpected findings: %v", got)
	}
}
