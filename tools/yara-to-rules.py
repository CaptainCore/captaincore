#!/usr/bin/env python3
"""Translate a YARA rule file into a CaptainCore scanner drop-in rule file.

Only the subset the webshell sets actually use is handled: text strings with
fullword / nocase / ascii / wide modifiers, and conditions of the form
"all of them", "any of them" or "N of them", optionally prefixed by filesize
or uintNN(0) checks (which are dropped). Rules with hex strings carrying bytes
above 0x7f, regex strings, or other conditions are skipped and listed, as are
rules whose names mark them as ASP, JSP or ColdFusion only.

Usage:
  tools/yara-to-rules.py thor-webshells.yar \
      --source "Neo23x0/signature-base thor-webshells.yar @94a1c48" \
      --license "DRL 1.1" --prefix sb --severity high \
      > lib/malware-signatures.d/signature-base-webshells.json
"""
import argparse, json, re, sys

ap = argparse.ArgumentParser()
ap.add_argument("yara")
ap.add_argument("--source", required=True)
ap.add_argument("--license", required=True)
ap.add_argument("--prefix", default="yara")
ap.add_argument("--severity", default="high")
ap.add_argument("--family", default="hacktool")
ap.add_argument("--skip-name", default=r"(?i)asp|jsp|cfm|servlet|war_|coldfusion|\.net")
ap.add_argument("--file-types", default="php")
ap.add_argument("--skip-file", help="file of rule names to leave out (one per line, # comments), e.g. rules that fired on clean WordPress trees")
args = ap.parse_args()

text = open(args.yara, encoding="utf-8", errors="replace").read()
text = re.sub(r"/\*.*?\*/", "", text, flags=re.S)
text = re.sub(r"^\s*//.*$", "", text, flags=re.M)

rule_re = re.compile(r"^rule\s+([A-Za-z0-9_]+)\s*(?::\s*[A-Za-z0-9_ ]+)?\s*\{(.*?)^\}", re.S | re.M)
skip_name = re.compile(args.skip_name)
skip_list = set()
if args.skip_file:
    for line in open(args.skip_file):
        line = line.split("#", 1)[0].strip()
        if line: skip_list.add(line.lower())

def unescape(s):
    out = bytearray(); i = 0
    while i < len(s):
        c = s[i]
        if c == "\\" and i + 1 < len(s):
            n = s[i + 1]
            if n == "x" and i + 3 < len(s):
                out.append(int(s[i + 2:i + 4], 16)); i += 4; continue
            out.append({"n": 10, "t": 9, "r": 13, "\\": 92, '"': 34}.get(n, ord(n))); i += 2; continue
        out.append(ord(c)); i += 1
    return bytes(out)

def to_pattern(raw, mods):
    b = unescape(raw)
    if any(x > 0x7e or (x < 0x20 and x not in (9, 10, 13)) for x in b):
        return None
    s = b.decode("ascii")
    if len(s) < 4:
        return None  # too short to mean anything on its own
    pat = re.escape(s).replace("\\ ", " ")
    if "fullword" in mods:
        if re.match(r"\w", s): pat = r"\b" + pat
        if re.search(r"\w$", s): pat = pat + r"\b"
    if "nocase" in mods:
        pat = "(?i)" + pat
    return pat

rules, skipped = [], []
for m in rule_re.finditer(text):
    name, body = m.group(1), m.group(2)
    if skip_name.search(name):
        skipped.append((name, "non-php")); continue
    if name.lower() in skip_list:
        skipped.append((name, "skip-file")); continue
    meta = dict(re.findall(r'^\s*([a-z_]+)\s*=\s*"((?:[^"\\]|\\.)*)"', body.split("strings:")[0], re.M)) if "meta:" in body else {}
    if "strings:" not in body or "condition:" not in body:
        skipped.append((name, "no strings")); continue
    strings_blk = body.split("strings:", 1)[1].split("condition:", 1)[0]
    cond = body.split("condition:", 1)[1].strip()
    pats, dropped = [], 0
    for sm in re.finditer(r'^\s*\$[A-Za-z0-9_]*\s*=\s*(?:"((?:[^"\\]|\\.)*)"|(\{[^}]*\})|(/(?:[^/\\]|\\.)+/[a-z]*))\s*(.*)$', strings_blk, re.M):
        txt, hexs, rx, mods = sm.groups()
        if txt is not None:
            p = to_pattern(txt, mods.split())
            if p: pats.append(p)
            else: dropped += 1
        elif hexs is not None:
            hb = hexs.strip("{} ").split()
            if any(not re.fullmatch(r"[0-9a-fA-F]{2}", h) for h in hb):
                dropped += 1; continue
            bs = bytes(int(h, 16) for h in hb)
            p = to_pattern(bs.decode("latin1").replace("\\", "\\\\").replace('"', '\\"'), [])
            if p: pats.append(p)
            else: dropped += 1
        else:
            dropped += 1  # regex strings: not verified as RE2, skip
    total = len(pats) + dropped
    if not pats:
        skipped.append((name, "no usable strings")); continue
    # Condition: strip filesize / uintNN prefixes, then require a simple "of them".
    c = re.sub(r"\(?\s*(uint(?:8|16|32)\(\s*0\s*\)\s*==\s*0x[0-9a-fA-F]+|filesize\s*[<>]=?\s*[0-9]+\s*[KM]?B?)\s*and\s*", "", cond).strip().strip("()").strip()
    mm = re.fullmatch(r"(all|any|\d+)\s+of\s+(?:them|\(\$[a-z0-9_*]+\))", c)
    if not mm:
        skipped.append((name, "condition: " + cond.replace("\n", " ")[:60])); continue
    need = len(pats) if mm.group(1) == "all" else (1 if mm.group(1) == "any" else int(mm.group(1)))
    if mm.group(1) == "all" and dropped:
        skipped.append((name, "all-of with dropped strings")); continue
    if need > len(pats):
        skipped.append((name, "need %d of %d usable" % (need, len(pats)))); continue
    rule = {
        "id": args.prefix + "-" + name.lower(),
        "name": name.replace("_", " "),
        "family": args.family,
        "severity": args.severity,
        "description": meta.get("description", "").replace("\\\"", '"') or ("YARA rule " + name),
        "patterns": pats,
        "file_types": args.file_types.split(","),
        "exclude_paths": ["/tests/", "/Tests/", "node_modules/", "wordfence", "sucuri", "malcare", "wp-cerber", "ithemes", "gotmls", "anti-malware", "security-ninja", "wp-security-audit", "virusdie", "quttera", "wp-simple-firewall", "shield-security", "captaincore", "malware-hunt"],
        "source": args.source + (" (author: %s)" % meta["author"] if meta.get("author") else ""),
        "license": args.license,
    }
    if need > 1:
        rule["min_matches"] = need
    if need == len(pats) and len(pats) > 1:
        # Every string is required: the longest literal is a free prefilter.
        lits = [p for p in pats if not p.startswith("(?i)")]
        if lits:
            best = max(lits, key=len)
            lit = re.sub(r"\\(.)", r"\1", best.replace(r"\b", ""))
            if len(lit) >= 6:
                rule["prefilter"] = [lit]
    rules.append(rule)

out = {"version": 2, "source": args.source, "license": args.license,
       "note": "Generated by tools/yara-to-rules.py; edit the YARA source or the translator, not this file.",
       "rules": rules}
json.dump(out, sys.stdout, indent=1, ensure_ascii=False); print()
print("translated %d rules, skipped %d" % (len(rules), len(skipped)), file=sys.stderr)
from collections import Counter
for k, v in Counter(r.split(":")[0] for _, r in skipped).most_common(): print("  skipped %4d  %s" % (v, k), file=sys.stderr)
