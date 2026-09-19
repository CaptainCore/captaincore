#!/usr/bin/env python3
"""Translate PHP-Antimalware-Scanner (amwscan) definitions into a drop-in rule file.

Usage: tools/amwscan-to-rules.py <PHP-Antimalware-Scanner checkout> [out.json]

Reads definitions/src/{signatures,hashes,metadata}.json and writes
lib/malware-signatures.d/amwscan.json:

  - knownMalwareSha256   -> hash indicators (critical)
  - domains              -> one rule, literal patterns, one match is enough (high)
  - raw                  -> one rule per literal (high); these are split-string and
                            base64 fragments of executable keywords
  - regex                -> one rule per pattern that Go's RE2 accepts (high), with a
                            required literal pulled out of the pattern as a prefilter
                            whenever one can be found at the top level

Patterns RE2 rejects (lookarounds, backreferences, repeat counts over 1000,
possessive quantifiers) are dropped and counted. amwscan is GPL-3.0, so this file
stays a separately licensed data file (see LICENSE-amwscan.txt) and is never
compiled into the binary.

Needs the re2check helper: `go build -o /tmp/re2check ./tools/re2check`.
"""
import json, os, re, subprocess, sys

src = sys.argv[1]
out = sys.argv[2] if len(sys.argv) > 2 else os.path.join(os.path.dirname(__file__), "..", "lib", "malware-signatures.d", "amwscan.json")
# Optional: a literal\tfiles table from tools/litfreq over the clean store. With
# it, each regex gets its rarest required literal as the prefilter instead of
# its longest, and a rule whose rarest literal still appears in more than
# MAX_DF of clean files is dropped: it would run its regex on that share of
# every site, for a pattern the corpus rules already cover.
freq = {}
if len(sys.argv) > 3:
    for line in open(sys.argv[3]):
        lit, _, n = line.rstrip("\n").rpartition("\t")
        if lit:
            freq[lit] = int(n)
MAX_DF = 0.003
defs = os.path.join(src, "definitions", "src")
sig = json.load(open(os.path.join(defs, "signatures.json")))
hashes = json.load(open(os.path.join(defs, "hashes.json")))
meta = json.load(open(os.path.join(defs, "metadata.json")))
version = meta.get("version", "?")
SRC = f"marcocesarato/PHP-Antimalware-Scanner definitions {version}"
LIC = "GPL-3.0"
EX = ["/tests/", "/Tests/", "node_modules/", "wordfence", "sucuri", "malcare", "wp-cerber", "ithemes", "gotmls",
      "anti-malware", "security-ninja", "wp-security-audit", "virusdie", "quttera", "wp-simple-firewall",
      "shield-security", "captaincore", "malware-hunt", "bearmor", "securicheck", "php-malware-finder", "amwscan"]

META = set(r"\.^$|?*+()[]{}")
MIN_PREFILTER = 8  # shortest required literal a regex rule may ship with

def top_level_literals(pat, minlen=6):
    """Every run of plain characters at nesting depth 0 that is not made
    optional by a following quantifier, longest first. Empty when a top-level
    alternation makes nothing required."""
    found = []
    cur = ""
    depth = 0
    i = 0
    n = len(pat)
    def flush(nxt):
        nonlocal cur
        # a quantifier right after the run makes its last char optional/repeated: drop that char
        if nxt in "?*{":
            cur = cur[:-1]
        if len(cur) >= minlen:
            found.append(cur)
        cur = ""
    while i < n:
        c = pat[i]
        if c == "\\":
            esc = pat[i + 1] if i + 1 < n else ""
            if depth == 0 and esc and (esc in META or esc in "/-"):
                cur += esc
                # a quantifier after an escaped literal char applies to that char
                if i + 2 < n and pat[i + 2] in "?*{":
                    cur = cur[:-1]
                    flush("")
            else:
                # \s, \d, \w and friends end the run; any quantifier after them is theirs
                flush("")
            i += 2
            continue
        if c == "[":
            flush(c)
            # skip the class
            j = i + 1
            if j < n and pat[j] == "^":
                j += 1
            if j < n and pat[j] == "]":
                j += 1
            while j < n and pat[j] != "]":
                if pat[j] == "\\":
                    j += 1
                j += 1
            i = j + 1
            # a quantifier after the class does not affect the literal before it
            continue
        if c == "(":
            flush(c)
            depth += 1
            i += 1
            if pat.startswith("?", i):
                # (?i) style flags or non-capturing groups
                while i < n and pat[i] != ")" and pat[i] != ":":
                    i += 1
                if i < n and pat[i] == ")":
                    depth -= 1
                i += 1
            continue
        if c == ")":
            flush(c)
            depth = max(0, depth - 1)
            i += 1
            continue
        if c == "|":
            flush(c)
            if depth == 0:
                return []  # a top-level alternation has no required literal
            i += 1
            continue
        if c in META:
            flush(c)
            i += 1
            continue
        if depth == 0:
            cur += c
        else:
            cur = ""
        i += 1
    flush("")
    return sorted(set(found), key=len, reverse=True)

def top_level_literal(pat):
    lits = top_level_literals(pat, 4)
    return lits[0] if lits else ""

def re2_ok(patterns):
    p = subprocess.run(["/tmp/re2check"], input="\n".join(patterns) + "\n", capture_output=True, text=True)
    return [l == "ok" for l in p.stdout.splitlines()]

rules = []
# domains
domains = [d for d in sig.get("domains", []) if re.match(r"^[a-z0-9.-]+$", d)]
if domains:
    rules.append({"id": "amw-known-malware-domain", "name": "Known malware domain", "family": "backdoor", "severity": "high",
                  "description": "A domain from the amwscan list of hosts used by web shells and injectors",
                  "patterns": ["(?i)\\b" + re.escape(d) + "\\b" for d in domains], "source": SRC, "license": LIC,
                  "exclude_paths": EX + ["vendor/"]})
# The raw list is a word list (".ssh/authorized_keys", "ls -la", "cmd.exe",
# eight-character base64 fragments): 25 of its 335 entries fired on clean
# vendor code in the first precision gate and none added recall. Off by
# default; RAW=1 in the environment brings it back for experiments.
SKIP = {
    "amw-re-03124",  # rot13 of base64_decode: CleanTalk and WP Migrate rot13 their own strings
    "amw-re-02984",  # curl_setopt + http_build_query: every HTTP client
}
for i, lit in enumerate(sig.get("raw", []) if os.environ.get("RAW") else []):
    if len(lit) < 6 or (freq and freq.get(lit, 0) > MAX_DF * 287834):
        continue
    rules.append({"id": f"amw-raw-{i:03d}", "name": "amwscan raw signature " + lit[:40], "family": "obfuscation", "severity": "high",
                  "description": "Literal fragment from the amwscan raw signature list (split strings and base64 of executable keywords)",
                  "patterns": [re.escape(lit)], "prefilter": [lit], "source": SRC, "license": LIC, "exclude_paths": EX + ["vendor/", ".min.js", "/build/", "/dist/"]})
# regexes
total_files = max(freq.values()) * 1.0 if freq else 0  # "function"-class literals bound the file count from below
if freq:
    total_files = max(total_files, 287834)
regs = sig.get("regex", [])
ok = re2_ok(regs)
kept = dropped = 0
for i, (pat, good) in enumerate(zip(regs, ok)):
    if not good or len(pat) < 12:
        dropped += 1
        continue
    r = {"id": f"amw-re-{i:05d}", "name": "amwscan signature " + str(i), "family": "backdoor", "severity": "high",
         "description": "Pattern from the amwscan regex signature list", "patterns": [pat], "source": SRC, "license": LIC,
         "exclude_paths": EX + ["vendor/"]}
    # The prefilter is a case-sensitive byte search, so a case-insensitive
    # pattern gets none.
    # A regex with no required literal, or only a short or common one, runs
    # against most files: 678 unprefiltered patterns made a 23k-file tree
    # take five times as long, and "function" is in two thirds of all files.
    cands = [] if "(?i" in pat else [l for l in top_level_literals(pat, MIN_PREFILTER)]
    if freq:
        cands = [l for l in cands if l in freq]
        cands.sort(key=lambda l: (freq[l], -len(l)))
        if cands and freq[cands[0]] > MAX_DF * total_files:
            cands = []
    if not cands or r["id"] in SKIP:
        dropped += 1
        continue
    r["prefilter"] = [cands[0]]
    # Signature regexes only match next to their literal; a 4 KB window each
    # side keeps Go's backtracker off the rest of a large file.
    r["window"] = 4096
    rules.append(r)
    kept += 1
hs = [{"sha256": h, "name": "amwscan known malware hash", "family": "backdoor", "severity": "critical",
       "description": "sha256 on the amwscan known-malware list", "source": SRC}
      for h in hashes.get("knownMalwareSha256", []) if re.match(r"^[0-9a-f]{64}$", h)]
doc = {"version": 2, "source": SRC, "license": LIC,
       "note": "Generated by tools/amwscan-to-rules.py; edit the translator, not this file. GPL-3.0 data file, kept separate from the MIT binary.",
       "rules": rules, "hashes": hs}
json.dump(doc, open(out, "w"), indent=1, ensure_ascii=False)
print(f"rules {len(rules)} (regex kept {kept}, dropped {dropped}; raw {sum(1 for r in rules if r['id'].startswith('amw-raw'))}; domains {len(domains)}), hashes {len(hs)} -> {out}")
print("prefilters:", sum(1 for r in rules if r.get('prefilter')), "of", len(rules))
