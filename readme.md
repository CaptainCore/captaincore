<h1 align="center">
  <a href="https://captaincore.io"><img src="https://captaincore.io/wp-content/uploads/2021/05/logo-white-2-300x300.png" width="70" /></a><br />
CaptainCore CLI
</h1>

CaptainCore is a WordPress management toolkit for hosting professionals. Built with [Cobra](https://cobra.dev) in Go, it manages WordPress sites at scale via SSH and WP-CLI.

## Features

- **Backups** -- Incremental backups with Restic to B2 cloud storage
- **Quicksaves** -- Git-based change tracking for plugins, themes, and config
- **Uptime Monitoring** -- Parallel HTTP checks with SQLite-backed stats and email alerts
- **SSH Management** -- Direct SSH and remote script execution across sites
- **WordPress Updates** -- Automated core, plugin, and theme updates with before/after diffs
- **Provider Integrations** -- Kinsta and GridPane hosting provider APIs
- **HTTP Server** -- REST + WebSocket API for remote command dispatch

## Project Structure

```
~/.captaincore/
├── captaincore.go          # Entry point
├── cmd/                    # Cobra command definitions
├── models/                 # GORM data models (SQLite)
├── config/                 # Config file parsing
├── providers/              # Hosting provider integrations (Kinsta, GridPane)
├── apiclient/              # HTTP client for CaptainCore API
├── server/                 # HTTP server (REST + WebSocket)
├── app/                    # Bash scripts for operations
├── lib/
│   ├── remote-scripts/     # Bash scripts executed on sites via SSH
│   ├── arguments           # Argument parsing helpers
│   └── excludes            # Backup exclusion rules
├── config.json             # CLI configuration (see config-sample.json)
├── data/                   # Runtime data (databases, keys, payloads)
├── go.mod / go.sum
├── install.sh              # curl | bash installer
└── .goreleaser.yml         # Release builds (linux/darwin, amd64/arm64/armv7)
```

## Requirements

- [Rclone](https://rclone.org) -- cloud storage sync
- [Restic](https://restic.net) -- encrypted backups
- Git

## Installation

```bash
# Linux and macOS
curl -fsSL https://raw.githubusercontent.com/CaptainCore/captaincore/master/install.sh | bash
```

The installer downloads the release build for your platform (Linux or macOS on x86_64, arm64 or armv7), verifies it against the release checksums and installs it as `/usr/local/bin/captaincore`. The binary carries its own runtime scripts and unpacks them into `~/.captaincore/` the first time it runs, so there is nothing else to clone. Set `INSTALL_DIR` to install somewhere else, or `CAPTAINCORE_VERSION=v1.0.0` to pin a release.

Then connect it to your CaptainCore Manager site (a WordPress site with pretty permalinks enabled and the [CaptainCore Manager](https://github.com/CaptainCore/captaincore-manager) plugin active). One command pairs both directions: it fetches the Manager's CLI token, and with `--server-url` it registers this machine's address so the Manager can dispatch jobs to `captaincore server`. No wp-config constants needed.

```bash
captaincore connect --server-url=https://captaincore-api.example.com
```

Keep it current with the built-in updater, which checks GitHub for a newer release, verifies the download and swaps the binary in place:

```bash
captaincore upgrade          # install the latest release
captaincore upgrade --check  # only report, exit 1 when behind
```

Releases are published at https://github.com/CaptainCore/captaincore/releases for manual download.

To build from source (requires Go 1.25+):

```bash
git clone https://github.com/CaptainCore/captaincore.git ~/.captaincore/
cd ~/.captaincore
go build -o captaincore .
```

A source checkout owns its own `app/` and `lib/` scripts. `captaincore upgrade` and `install.sh` detect the `.git` directory and leave it alone; update a checkout with `git pull` and a rebuild.

## Configuration

`captaincore connect` writes `config.json` for you. To configure by hand, copy `config-sample.json` to `config.json` and set:

- **System config** -- server paths, master SSH connection, fleet mode
- **Per-tenant config** -- API keys, cloud storage remotes (rclone), branding, site list

## Usage

```bash
# Site operations
captaincore site list
captaincore site get <site>
captaincore ssh <site>

# Backups
captaincore backup generate <site>
captaincore backup list <site>
captaincore backup download <site>

# Quicksaves (change tracking)
captaincore quicksave generate <site>
captaincore quicksave list <site>
captaincore quicksave show-changes <site>

# Monitoring
captaincore monitor

# Updates
captaincore update <site>

# Bulk operations (target groups)
captaincore backup generate @all
captaincore update @production

# Fleet mode (multi-tenant)
captaincore backup generate @all --fleet

# Start HTTP server
captaincore server
```

Run `captaincore --help` for a full list of commands.

## Releasing

```bash
git tag -a v1.0.0 -m "v1.0.0"
git push origin v1.0.0
goreleaser release --snapshot --skip-publish --clean    # Preview
goreleaser release --skip-validate --clean              # Publish
```

## Documentation

Refer to [docs.captaincore.io](https://docs.captaincore.io) for full documentation.

## License

MIT License. See [license](license) for details.
