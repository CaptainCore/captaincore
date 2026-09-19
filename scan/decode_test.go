package scan

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"path/filepath"
	"testing"
)

func deflate(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, flate.BestCompression)
	w.Write(b)
	w.Close()
	return buf.Bytes()
}

const decodePayload = "<?php if(isset($_POST['cmd'])){ eval($_POST['cmd']); } // padding to make the run long enough for the decoder to care about it"

func TestDecodeLayers(t *testing.T) {
	payload := []byte(decodePayload)
	b64 := base64.StdEncoding.EncodeToString(payload)
	deflated := base64.StdEncoding.EncodeToString(deflate(t, payload))
	inner := base64.StdEncoding.EncodeToString([]byte("<?php $x = 1; " + decodePayload))
	nested := base64.StdEncoding.EncodeToString(deflate(t, []byte("<?php eval(base64_decode('"+inner+"'));")))
	unescaped := "<?php $f = \"\\x65\\x76\\x61\\x6c\"; $f(\"\\x24\\x5f\\x50\\x4f\\x53\\x54\"); eval($_POST[\"\\x63\"]); // \\x70\\x61\\x64\\x64\\x69\\x6e\\x67 padding padding"

	cases := map[string]struct{ file, layer string }{
		"base64":           {"<?php eval(base64_decode('" + b64 + "'));", "base64"},
		"base64+gzinflate": {"<?php eval(gzinflate(base64_decode('" + deflated + "')));", "base64+gzinflate"},
		"nested":           {"<?php eval(gzinflate(base64_decode('" + nested + "')));", "base64+gzinflate+base64"},
		"rot13":            {"<?php eval(str_rot13('" + string(rot13(payload)) + "'));", "rot13"},
		"unescape":         {unescaped, "unescape"},
	}
	// The unescape layer needs a decoder marker too now; hex2bin is one.
	cases["unescape"] = struct{ file, layer string }{unescaped + " hex2bin('00');", "unescape"}
	for name, c := range cases {
		layers := decodeLayers([]byte(c.file))
		found := false
		var got []string
		for _, l := range layers {
			got = append(got, l.Layer)
			if l.Layer == c.layer && bytes.Contains(l.Data, []byte("eval(")) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: layer %q not produced, got %v", name, c.layer, got)
		}
	}
	binary := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0xff, 0x00, 0x13}, 40))
	if n := len(decodeLayers([]byte("<?php eval(base64_decode('" + binary + "'));"))); n != 0 {
		t.Errorf("binary base64 content must not produce a layer, got %d", n)
	}
	// No decoder call in the file: the layer stays off, however long the blob.
	if n := len(decodeLayers([]byte("<?php $font = '" + b64 + "';"))); n != 0 {
		t.Errorf("a blob without a decoder must not be decoded, got %d layers", n)
	}
}

func TestDecodedPayloadsAreScanned(t *testing.T) {
	s := shippedRules(t)
	dir := t.TempDir()
	payload := "<?php if(isset($_POST['cmd'])){ eval($_POST['cmd']); } // FilesMan padding padding padding padding"
	b64 := base64.StdEncoding.EncodeToString(deflate(t, []byte(payload)))
	// Nothing in the raw file names a request superglobal or a shell; only the payload does.
	p := write(t, dir, "plugins/x/cache.php", "<?php $k = '"+b64+"'; $z = gzinflate(base64_decode($k)); $z();")
	f, err := s.ScanFile(p, "plugins/x/cache.php")
	if err != nil {
		t.Fatal(err)
	}
	var layered []string
	for _, x := range f {
		if x.Layer != "" {
			layered = append(layered, x.RuleID+"@"+x.Layer)
		}
	}
	if len(layered) == 0 {
		t.Fatalf("expected findings from the decoded payload, got %v", ids(f))
	}
	want := map[string]bool{"eval-request-input@base64+gzinflate": false, "webshell-names@base64+gzinflate": false}
	for _, l := range layered {
		if _, ok := want[l]; ok {
			want[l] = true
		}
	}
	for k, ok := range want {
		if !ok {
			t.Errorf("missing %s in %v", k, layered)
		}
	}
	for _, x := range f {
		if x.Layer != "" && x.Legacy().SignatureDescription == x.Description {
			t.Errorf("legacy description should mention the layer: %+v", x.Legacy())
		}
	}
	// NoDecode turns the layer off.
	rs, _ := LoadRuleSet(filepath.Join("..", "lib", "malware-signatures.json"))
	s2 := New(rs, Options{NoDecode: true})
	f2, _ := s2.ScanFile(p, "plugins/x/cache.php")
	for _, x := range f2 {
		if x.Layer != "" {
			t.Errorf("NoDecode still produced a layered finding: %+v", x)
		}
	}
}
