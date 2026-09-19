package scan

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"encoding/base64"
	"io"
	"regexp"
	"strconv"
)

// Decoding layer: signatures written against obfuscated text say nothing
// about what the payload does. Before giving up on a file, the scanner
// decodes what it can (base64 runs, deflate/zlib/gzip inside them, rot13 when
// the file calls str_rot13, \xNN escapes) and runs the same rules over the
// result. A finding made this way carries Layer, e.g. "base64+gzinflate".

// MaxDecodedLayers is how many decoded payloads per file are scanned.
const MaxDecodedLayers = 6

// minBase64Run is the shortest base64 run worth decoding.
const minBase64Run = 64

var (
	base64Run = regexp.MustCompile(`[A-Za-z0-9+/]{` + strconv.Itoa(minBase64Run) + `,}={0,2}`)
	hexEscape = regexp.MustCompile(`\\x[0-9a-fA-F]{2}`)
	octEscape = regexp.MustCompile(`\\[0-7]{3}`)
)

// decodedPayload is one candidate derived from the file content.
type decodedPayload struct {
	Layer string
	Data  []byte
}

// looksLikeCode reports whether decoded bytes are worth matching: mostly
// printable text with something PHP- or script-shaped in it.
func looksLikeCode(b []byte) bool {
	if len(b) < 16 {
		return false
	}
	printable := 0
	for _, c := range b {
		if c == '\n' || c == '\r' || c == '\t' || (c >= 0x20 && c < 0x7f) {
			printable++
		}
	}
	if printable*100 < len(b)*85 {
		return false
	}
	for _, marker := range [][]byte{[]byte("<?"), []byte("$_"), []byte("eval"), []byte("function"), []byte("base64"), []byte("<script"), []byte("<a "), []byte("http")} {
		if bytes.Contains(b, marker) {
			return true
		}
	}
	return false
}

func tryInflate(b []byte) ([]byte, string) {
	limit := int64(DefaultMaxBytes)
	if r := flate.NewReader(bytes.NewReader(b)); r != nil {
		if out, err := io.ReadAll(io.LimitReader(r, limit)); err == nil && len(out) > 0 {
			return out, "gzinflate"
		}
	}
	if r, err := zlib.NewReader(bytes.NewReader(b)); err == nil {
		if out, err := io.ReadAll(io.LimitReader(r, limit)); err == nil && len(out) > 0 {
			return out, "gzuncompress"
		}
	}
	if r, err := gzip.NewReader(bytes.NewReader(b)); err == nil {
		if out, err := io.ReadAll(io.LimitReader(r, limit)); err == nil && len(out) > 0 {
			return out, "gzdecode"
		}
	}
	return nil, ""
}

func rot13(b []byte) []byte {
	out := make([]byte, len(b))
	for i, c := range b {
		switch {
		case c >= 'a' && c <= 'z':
			out[i] = 'a' + (c-'a'+13)%26
		case c >= 'A' && c <= 'Z':
			out[i] = 'A' + (c-'A'+13)%26
		default:
			out[i] = c
		}
	}
	return out
}

func unescapeHex(b []byte) []byte {
	out := hexEscape.ReplaceAllFunc(b, func(m []byte) []byte {
		v, _ := strconv.ParseUint(string(m[2:]), 16, 8)
		return []byte{byte(v)}
	})
	return octEscape.ReplaceAllFunc(out, func(m []byte) []byte {
		v, _ := strconv.ParseUint(string(m[1:]), 8, 8)
		return []byte{byte(v)}
	})
}

// MaxDecodeBytes is the largest file the decoding layer looks at.
const MaxDecodeBytes = 1024 * 1024

// decoderMarkers are the calls a file needs before its blobs are worth
// decoding: a payload is inert without something to unpack it, and this
// keeps the layer off the vast majority of files.
var decoderMarkers = [][]byte{
	[]byte("base64_decode"), []byte("gzinflate"), []byte("gzuncompress"), []byte("gzdecode"),
	[]byte("str_rot13"), []byte("hex2bin"), []byte("strrev"), []byte("convert_uu"), []byte("pack("),
}

// worthDecoding reports whether the decoding layer should run on data. The
// raw rules already catch eval(base64_decode(...)) shapes; decoding is for
// what the payload does, which needs a decoder call in the file.
func worthDecoding(data []byte) bool {
	if len(data) > MaxDecodeBytes {
		return false
	}
	for _, m := range decoderMarkers {
		if bytes.Contains(data, m) {
			return true
		}
	}
	return false
}

// decodeLayers returns the payloads hidden in data, most promising first,
// capped at MaxDecodedLayers. It is a no-op unless worthDecoding.
func decodeLayers(data []byte) []decodedPayload {
	if !worthDecoding(data) {
		return nil
	}
	var out []decodedPayload
	seen := map[string]bool{}
	add := func(layer string, b []byte) bool {
		if len(out) >= MaxDecodedLayers || !looksLikeCode(b) {
			return false
		}
		key := layer + ":" + strconv.Itoa(len(b)) + ":" + string(b[:min(len(b), 64)])
		if seen[key] {
			return false
		}
		seen[key] = true
		out = append(out, decodedPayload{Layer: layer, Data: b})
		return true
	}

	for _, run := range base64Run.FindAll(data, 12) {
		if len(out) >= MaxDecodedLayers {
			break
		}
		dec, err := base64.StdEncoding.DecodeString(string(run))
		if err != nil {
			dec, err = base64.RawStdEncoding.DecodeString(string(bytes.TrimRight(run, "=")))
			if err != nil {
				continue
			}
		}
		if inflated, how := tryInflate(dec); inflated != nil {
			add("base64+"+how, inflated)
			// A second base64 layer inside the inflated payload is common.
			for _, inner := range base64Run.FindAll(inflated, 4) {
				if d2, err := base64.StdEncoding.DecodeString(string(inner)); err == nil {
					if i2, how2 := tryInflate(d2); i2 != nil {
						add("base64+"+how+"+base64+"+how2, i2)
					} else {
						add("base64+"+how+"+base64", d2)
					}
				}
			}
			continue
		}
		if add("base64", dec) {
			for _, inner := range base64Run.FindAll(dec, 4) {
				if d2, err := base64.StdEncoding.DecodeString(string(inner)); err == nil {
					if i2, how2 := tryInflate(d2); i2 != nil {
						add("base64+base64+"+how2, i2)
					} else {
						add("base64+base64", d2)
					}
				}
			}
		}
	}
	if bytes.Contains(data, []byte("str_rot13")) {
		add("rot13", rot13(data))
	}
	if hexEscape.Match(data) || octEscape.Match(data) {
		if un := unescapeHex(data); !bytes.Equal(un, data) {
			add("unescape", un)
		}
	}
	return out
}
