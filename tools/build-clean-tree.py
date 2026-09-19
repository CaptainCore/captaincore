#!/usr/bin/env python3
"""Relink the deduplicated clean store into a tree that keeps each file's original
relative path (site_dir/env/quicksave/...), so path-scoped rules (exclude_paths,
include_paths) behave exactly as they do on a real quicksave. Uses the manifest,
no hashing: 287k hard links take about a minute. Store layout stays as is."""
import os, sys
C = sys.argv[1]
STORE, TREE = os.path.join(C, "clean"), os.path.join(C, "clean-tree")
n = 0; missing = 0
for l in open(os.path.join(C, "clean-manifest.tsv")):
    sha, p = l.rstrip("\n").split("\t", 1)
    rel = p.split("/Sites/1/", 1)[-1]
    low = os.path.basename(p).lower()
    ext = os.path.splitext(low)[1] if low != ".htaccess" else ".htaccess"
    if ".php." in low: ext = ".php"
    src = os.path.join(STORE, sha[:2], sha + ext)
    dst = os.path.join(TREE, rel)
    if not os.path.exists(src):
        missing += 1; continue
    os.makedirs(os.path.dirname(dst), exist_ok=True)
    try:
        os.link(src, dst); n += 1
    except FileExistsError:
        pass
print(f"linked {n} files into {TREE}; missing {missing}")
