package main

import "embed"

// runtimeAssets carries the bash and PHP scripts the binary execs at runtime
// (app/) and the remote scripts, signature files and exclude lists (lib/).
// cmd.ensureAssets materializes them into ~/.captaincore on a binary-only
// install so install.sh and `captaincore upgrade` only ever move one file.
//
//go:embed all:app all:lib
var runtimeAssets embed.FS
