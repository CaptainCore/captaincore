#!/usr/bin/env python3
"""Usage: build-clean-store.py [clean-trees.txt] [corpus dir]   (run on the core server)

Build a deduplicated store of known-clean files for fast precision gates.

Walks the trees listed in clean-trees.txt (sites whose security audit is clean
and never remediated), hashes every scannable file, and hard-links each unique
file once into <corpus>/clean/<sha256 prefix>/<sha256>.<ext>. Hard links cost
no extra space (same filesystem) and keep the extension so file-type rules
apply. A manifest maps each stored file to one original path for reporting.
"""
import hashlib, os, sys, collections

LIST = sys.argv[1] if len(sys.argv) > 1 else os.path.expanduser("~/Scripts/wordfence-harvest/clean-trees.txt")
C = sys.argv[2] if len(sys.argv) > 2 else "/mnt/disks/storage/wordfence-corpus"
STORE = os.path.join(C, "clean")
EXTS = {".php", ".phtml", ".php5", ".php7", ".inc", ".js", ".mjs", ".html", ".htm", ".svg", ".txt", ".json", ".css", ".log", ".ico", ".htaccess"}
MAX = 8 * 1024 * 1024

os.makedirs(STORE, exist_ok=True)
seen = set()
if os.path.exists(os.path.join(C, "clean-manifest.tsv")):
    for l in open(os.path.join(C, "clean-manifest.tsv")):
        seen.add(l.split("\t", 1)[0])
manifest = open(os.path.join(C, "clean-manifest.tsv"), "a")
files = 0; unique = 0; per_ext = collections.Counter()
for tree in [l.strip() for l in open(LIST) if l.strip()]:
    for root, dirs, names in os.walk(tree):
        dirs[:] = [d for d in dirs if d not in (".git", "node_modules")]
        for n in names:
            low = n.lower()
            ext = os.path.splitext(low)[1] if low != ".htaccess" else ".htaccess"
            if ".php." in low:
                ext = ".php"
            if ext not in EXTS:
                continue
            p = os.path.join(root, n)
            try:
                st = os.stat(p)
                if st.st_size == 0 or st.st_size > MAX or not os.path.isfile(p):
                    continue
                h = hashlib.sha256()
                with open(p, "rb") as f:
                    for chunk in iter(lambda: f.read(1 << 20), b""):
                        h.update(chunk)
            except OSError:
                continue
            files += 1
            sha = h.hexdigest()
            if sha in seen:
                continue
            seen.add(sha); unique += 1; per_ext[ext] += 1
            d = os.path.join(STORE, sha[:2]); os.makedirs(d, exist_ok=True)
            dst = os.path.join(d, sha + ext)
            try:
                os.link(p, dst)
            except FileExistsError:
                pass
            except OSError:
                with open(p, "rb") as src, open(dst, "wb") as out:
                    out.write(src.read())
            manifest.write(f"{sha}\t{p}\n")
    print(f"{tree.split('/Sites/1/')[-1]}: files so far {files}, unique {unique}", flush=True)
manifest.close()
print(f"DONE files={files} unique={unique} {dict(per_ext.most_common(8))}")
