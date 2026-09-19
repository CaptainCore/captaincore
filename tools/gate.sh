#!/bin/bash
# Local recall + precision gate for the malware scanner.
#
#   tools/gate.sh            recall over the corpus samples, precision over the clean store
#   tools/gate.sh recall     recall only (seconds)
#   tools/gate.sh precision  precision only
#
# Corpus layout (mirrored from the core server, never committed):
#   $CORPUS/samples/<sha256>          Wordfence-confirmed malicious files
#   $CORPUS/index.tsv                 site, env, path, sha256, size, sig id, sig name, family, description, matched text
#   $CORPUS/negatives.txt             sha256 of samples that are Wordfence false positives
#   $CORPUS/clean/<xx>/<sha256>.<ext> unique files from sites whose security audit is clean
#   $CORPUS/clean-manifest.tsv        sha256 -> one original quicksave path
#   $CORPUS/clean-tree/<site>/<env>/quicksave/...  the same files hard-linked under their
#                                     original paths, so path-scoped rules behave as on a site
#
# Recall counts a sample as caught when any rule fires at high or critical.
# Precision lists every high or critical hit on the clean store: each is a
# false positive to fix (or a negative to record) before the rules ship.
set -uo pipefail
CORPUS=${CORPUS:-$HOME/Documents/wordfence-corpus}
REPO=$(cd "$(dirname "$0")/.." && pwd)
MODE=${1:-all}
BIN=${BIN:-/tmp/captaincore-gate}
RULES=(--rules="$REPO/lib/malware-signatures.json")
for f in "$REPO"/lib/malware-signatures.d/*.json; do [ -f "$f" ] && RULES+=(--rules="$f"); done

(cd "$REPO" && go build -o "$BIN" ./) || exit 1

if [ "$MODE" = all ] || [ "$MODE" = recall ]; then
  T=$(mktemp -d /tmp/cc-gate.XXXX); mkdir -p "$T/plugins/corpus"
  for h in "$CORPUS"/samples/*; do cp "$h" "$T/plugins/corpus/$(basename "$h").php"; done
  "$BIN" scan "${RULES[@]}" --format=json --workers=4 "$T" 2>/dev/null > /tmp/cc-gate-recall.json
  python3 - "$CORPUS" <<'PY'
import json, os, sys, collections
C=sys.argv[1]
neg=set(l.split("#")[0].strip() for l in open(C+"/negatives.txt") if l.strip() and not l.startswith("#"))
fam={}
for l in open(C+"/index.tsv"):
    c=l.rstrip("\n").split("\t"); fam.setdefault(c[3], c[6])
hits=collections.defaultdict(set)
for x in (json.load(open("/tmp/cc-gate-recall.json")) or []):
    hits[x["file"].split("/")[-1][:-4]].add(x["severity"])
pos=[s for s in sorted(os.listdir(C+"/samples")) if s not in neg]
caught=[s for s in pos if hits[s] & {"high","critical"}]
print(f"RECALL {len(caught)}/{len(pos)} ({len(neg)} negatives excluded)")
for s in pos:
    if s not in caught: print("  MISS", s[:10], fam.get(s,"?"))
PY
  rm -rf "$T"
fi

if [ "$MODE" = all ] || [ "$MODE" = precision ]; then
  STORE="$CORPUS/clean-tree"; [ -d "$STORE" ] || STORE="$CORPUS/clean"
  [ -d "$STORE" ] || { echo "no clean store at $STORE"; exit 1; }
  t0=$(date +%s)
  "$BIN" scan "${RULES[@]}" --format=json --workers=6 "$STORE" 2>/dev/null > /tmp/cc-gate-precision.json
  python3 - "$CORPUS" "$REPO" "$(( $(date +%s)-t0 ))" "$STORE" <<'PY'
import json, os, sys, collections, glob
C, REPO, secs, STORE = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
sev={}
for rf in [REPO+"/lib/malware-signatures.json"]+sorted(glob.glob(REPO+"/lib/malware-signatures.d/*.json")):
    for r in json.load(open(rf))["rules"]: sev[r["id"]]=r["severity"]
orig={}
for l in open(C+"/clean-manifest.tsv"):
    h,p=l.rstrip("\n").split("\t",1); orig[h]=p.split("/Sites/1/")[-1]
rows=json.load(open("/tmp/cc-gate-precision.json")) or []
n=sum(len(fs) for _,_,fs in os.walk(STORE))
hi=[x for x in rows if sev.get(x["rule_id"]) in ("high","critical")]
print(f"PRECISION {len(hi)} high/critical over {n} unique clean files in {secs}s ({len(rows)} total incl. low/medium)")
for k,v in collections.Counter(x["rule_id"] for x in rows).most_common(): print(f"  {v:4d} {k} [{sev.get(k)}]")
seen=collections.defaultdict(int)
for x in hi:
    if seen[x["rule_id"]]<5:
        seen[x["rule_id"]]+=1; h=x["file"].split("/")[-1].split(".")[0]
        where = x["file"] if STORE.endswith("clean-tree") else orig.get(h, x["file"])
        print("  FP", x["rule_id"], "|", where[:110], "|", (x.get("match") or "")[:90].replace("\n"," "))
PY
fi
