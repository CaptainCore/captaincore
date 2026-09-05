# Changelog

## [1.0.0] - 2026-XX-XX

### Overview

CaptainCore CLI 1.0 runs on SQLite and no longer needs an embedded WordPress. Through 0.13 the CLI kept every site, environment, account and provider record inside a private local WordPress install and reached them by shelling out to WP-CLI and a folder of PHP scripts, so standing up the CLI meant standing up PHP, WP-CLI and a throwaway WordPress site first. Version 1.0 stores all of that in a local SQLite database at `~/.captaincore/data/captaincore.db` (GORM on the pure-Go `glebarez/sqlite` driver, so no cgo and no system SQLite), reads its settings through a native Go `config` package, and talks to the CaptainCore Manager over a plain Go HTTP client. The CaptainCore Manager plugin remains the source of truth the CLI syncs from; what went away is the local WordPress underneath the CLI.

Installation is now one static binary. `curl -fsSL https://raw.githubusercontent.com/CaptainCore/captaincore/master/install.sh | bash` puts it in place, `captaincore connect` points it at your Manager with a WordPress application password and pulls down every site, environment, account, domain and provider, and `captaincore upgrade` keeps it current from GitHub releases. The bash and remote scripts the CLI still uses at runtime ship embedded in the binary and unpack themselves on first run.

Along the way the CLI grew a full restic operations toolkit, fleet-wide version drift reporting, a git-backed quicksave malware scanner, long-term log archival, and 38 new remote scripts, most of them security detection.

### Breaking Changes

- **The private local WordPress install and WP-CLI are no longer read or supported.** Data lives in `~/.captaincore/data/captaincore.db`. Existing installs must run `captaincore connect` (or the one-time `captaincore migrate wp-to-sqlite`) before any data command works. Commands that need data exit with `Error: Database not available. Run 'captaincore connect'`.
- **`captaincore monitor <site|target>` is now `captaincore monitor run <site|target>`**, and **`captaincore capture <site|target>` is now `captaincore capture generate <site|target>`**. Update cron entries.
- **`fetch-site-data` emits `key:value` pairs** instead of positional lines. Anything parsing the old output breaks.
- **Backups now include WordPress core files.** Every core-file exclusion (`wp-admin/`, `wp-includes/`, `index.php`, `wp-login.php`, `xmlrpc.php` and friends) was removed from `lib/excludes` and `lib/restic-excludes`. Expect larger first backups after upgrading.
- **45 of the 51 PHP scripts in `lib/local-scripts/` were deleted** and 57 files were removed from `app/` (the `dns/`, `key/`, `cli/`, `get/` and `utils/` trees, `monitor`, `copy`, `configs`, `scan-errors`, `regenerate-thumbnails`, `manifest-generate` and the `*-generate` / `*-list` variants). Custom automation that called those files by path will break; the replacements are Cobra commands.
- **The `configs.php` config layer is retired.** `config.json` is read and written by Go and rewritten with mode `0600`. Bash callers use `captaincore config fetch` instead.
- **On a binary install the CLI owns `~/.captaincore/app` and `~/.captaincore/lib`** and rewrites them whenever the binary version changes (stamped in `.assets-version`). Local edits there do not survive an upgrade. A git checkout at that path is detected and left alone.
- **`backup download`** takes the payload as `--payload=<token>` (the third positional argument still works).

### New Features

**Install, connect, upgrade**

- **`install.sh`.** Detects Linux or macOS on x86_64, arm64 or armv7, downloads the release archive and `checksums.txt`, verifies the sha256 and installs to `/usr/local/bin` (`INSTALL_DIR`, `CAPTAINCORE_VERSION=vX.Y.Z` and `--force` are supported).
- **`captaincore connect [--url=] [--username=] [--password=] [--server-url=] [--skip-ssl]`.** Authenticates with a WordPress application password against `/wp-json/captaincore/v1/cli/connect`, upserts sites, environments, accounts, domains, junction rows, providers, configurations and defaults into SQLite, then writes the token, API URL, GUI URL and site list into `config.json` and the token into `data/config.json` for `captaincore server`. `--server-url` registers this server's public address with the Manager for job dispatch, so pairing both directions is one command and no wp-config constant is required (needs captaincore-manager newer than 1.1.0). A Manager still on plain permalinks is handled by falling back to the `?rest_route=` form, and a freshly connected Manager with zero sites counts as connected. A re-run shows an add/update/remove preview and asks before applying, then warns about a missing `rclone`, `restic` or `git`. `captaincore connect --sync` does the same non-interactively for cron.
- **`captaincore upgrade [--check] [--yes] [--force] [--version=vX.Y.Z]`.** Follows the GitHub `releases/latest` redirect (no API token), downloads the archive for this OS and CPU, verifies it against the release checksums, sanity-runs the new binary and swaps it in place, with a `sudo mv` fallback. `--check` exits 1 when behind. A source checkout is refused unless `--force`.
- **Embedded runtime scripts.** `app/` and `lib/` ship inside the binary (`go:embed`) and unpack into `~/.captaincore/` on the first run of each version. `config.json` and `data/` are never touched.
- **Release builds** for Linux and macOS on x86_64, arm64 and armv7 (`CGO_ENABLED=0`, static), with stable asset names under `releases/latest/download/`.
- **`captaincore migrate wp-to-sqlite [<site-id>...]`.** One-time backfill from a Manager for pre-1.0 installs.

**Local data layer**

- **SQLite with GORM.** Fourteen auto-migrated tables (`captaincore_sites`, `_environments`, `_accounts`, `_domains`, `_account_site`, `_account_domain`, `_account_user`, `_captures`, `_snapshots`, `_keys`, `_configurations`, `_recipes`, `_connections`, `_providers`), plus `data/monitor_stats.db` for monitor runs. Both database files are chmod `0600`. Tuned for parallel bulk runs: WAL journal, `busy_timeout=30000`, `synchronous=NORMAL`, one pooled connection so writes queue instead of racing, and a WAL checkpoint after bulk runs.
- **Native Go API client** (`apiclient/`) for every Manager call.
- **`captaincore config fetch [<section>] [<key>]`, `config fetch-captain-ids`, `config from-api [--field=]`**, and **`captaincore configuration get [--field=] [--bash]` / `configuration sync`**. **`captaincore cron`** prints the stored scheduled tasks as JSON.
- **`captaincore info [--json]`.** Version, platform, Manager and API URLs, and site, account, environment and domain counts.

**Hosting providers**

- **Provider framework** (`providers/`) with a registry. Ships **Kinsta** (`api_key`, `company_id`) and **GridPane** (`api_key`), each able to list remote sites and enrich a site with SSH user, host, port, password, home directory, home URL, WordPress version and monthly visits.
- **`captaincore provider add <name> <slug> --credentials='[{"name":"api_key","value":"..."}]'`, `provider list`, `provider update <id> [--credentials=] [--status=]`, `provider delete <id>`, `provider sync`, `provider remote-sites <id>`, `provider import <id> [--site-ids=] [--account-id=] [--update-extras]`.**
- **`captaincore connection add <domain> <domain-token> <captaincore-token>` and `connection list`** for CaptainCore-to-CaptainCore links.
- **`captaincore site ssh-refresh <site>`** pulls fresh SSH credentials from the provider; **`captaincore ssh-detect <user> <address> <port>`** and **`captaincore ssh-verify <conn>`** probe a connection.

**Backups and restic operations (native Go)**

- **Direct backup mode** is the default. `backup generate` pipes the `vault` script plus a temporary secrets payload over SSH and the managed host runs restic straight to B2; the old pull-to-local flow survives only as `backup_mode="local"`. The remote run hands the snapshot list back inline, so `backup list-generate` and `backup verify` never run restic locally. Flags: `--parallel=3`, `--skip-db`, `--skip-remote`, `--skip-if-recent=24h`, `--dry-run`.
- **Self-healing connections.** When rclone cannot reach a site, `backup generate` regenerates its rclone config, retries, then calls `site ssh-refresh` to pull fresh SSH credentials from the hosting provider. A broken `vault.txt` (missing bucket) is detected and its keys regenerated.
- **Repository maintenance:** `backup check <site> [--init] [--read-data]`, `backup verify <site>`, `backup snapshots <site> [id] [--sizes] [--format=json]`, `backup repo-info <site> [--stats]`, `backup find <site> <pattern>`, `backup forget <site> <snapshot-id> [--confirm] [--prune]`, `backup prune <site> [--dry-run] [--repack-uncompressed]`, `backup repair <site> [--packs] [--snapshots] [--forget]`, `backup unlock <site> [--remove-all]`.
- **Restic v2 migration:** `backup upgrade <site>` and `backup migrate-v2 <site> [--force] [--skip-repack] [--skip-cache-cleanup]` move repositories to the v2 format with compression.
- **Key safety:** `backup key-backup <site> [--type=backup|quicksave]` and `backup key-restore <site>` guard against losing the B2 repository key.
- **Housekeeping:** `backup cleanup <site> [--dry-run]` removes local `backup/` folders on remote-backup sites; `backup storage-cleanup [--confirm]` removes orphaned site folders from B2 (dry-run by default); `backup show <site> <backup-id> <file-id>`, `backup runtime <site>`, `backup fetch-link`.
- **Per-site PID locks.** `backup generate`, `prune` and `migrate-v2` take a `backup.lock` holding the PID (stale locks from dead processes are cleared silently) and pass `--retry-lock 30m` so concurrent runs wait or skip instead of colliding. Quicksaves use the same `quicksave.lock` mechanism.

**Quicksaves**

- **`quicksave malware-scan <site|@target> [--full] [--format=json]`** runs the signature set in `lib/malware-signatures.json` over the git history; `--full` runs Wordfence CLI across the whole quicksave directory.
- **Remote fingerprint fast path.** `quicksave generate` first asks the site for a content hash (`quicksave-fingerprint`); when it matches the stored `.fingerprint` and `--force` is absent, the three rclone syncs are skipped entirely. `quicksave add` still runs so core-checksum drift in root files is caught. `vuln-scan` and `quicksave backup` run only when the quicksave hash actually changed.
- **`quicksave backup <site> [--parallel=10] [--skip-if-recent=24h]`** pushes the git repository into a restic `quicksave-repo` and purges the local restic cache afterwards; **`quicksave restore-git <site>`** brings it back (refusing to clobber an existing `.git`) and rebuilds the JSON cache.
- **`quicksave archive <site> <hash> [--plugin=|--theme=]`** extracts a plugin or theme zip from any commit; **`quicksave database <site> <hash>`** extracts and sanitizes the SQL from the nearest backup snapshot.
- **`quicksave add <site> [--force]`, `quicksave latest <site> [--field=]`, `quicksave search <site> <theme|plugin:title|name:term>`, `quicksave list --field=`, `quicksave cache-check`, `quicksave cache-purge [--dry-run]`, `quicksave unlock`, `quicksave migrate-v2`.** `show-changes` takes an optional match filter; `rollback` gained `--plugin`, `--theme`, `--file`, `--all` (wrapped in maintenance mode) and `--version=previous`.
- `quicksave generate` gained `--parallel=10`, `--skip-if-recent`, `--dry-run`, `--force`, lock checking, loose-file tracking and core-version detection.

**Fleet visibility**

- **`captaincore drift [--plugin=|--theme=|--core|--themes] [--target=latest] [--hashes] [--provider=] [--environment=] [--top=20] [--sort=volume|spread] [--json]`** shows how a component's versions are spread across the fleet. **`drift --steer --force [--parallel=10]`** upgrades every drifted site. **`drift diff --plugin=<slug> [--hash=] [--summary]`** shows real file-level differences between hash variants, sourced from local quicksaves.
- **`captaincore capture scan <site|@target> [--filter=critical|warning|external] [--malware] [--format=json]`** scans captured HTML for injected scripts and styles; **`capture check <site>`** flags new injections in the latest capture.
- **`captaincore site orphans [--write-list=] [--from-list=] [--confirm] [--include-stale-names]`** finds `{site}_{id}` folders with no active site. Dry-run by default; deletes are confined to `system.path` and re-checked at delete time.
- **`captaincore site search <term> [--field=] [--search-field=]`, `site vuln-scan <site> [--cached]`, `site sync-batch <site-id>... [--update-extras]`.**
- **Monitor rewritten in Go.** The bash `monitor` and `monitor-check` scripts are gone. `captaincore monitor run <site|@target> [--parallel=10] [--retry=3] [--page=]` uses a bounded worker pool with DNS fallback, records runs in `monitor_stats.db`, and sends an HTML digest; `monitor-check` and `monitor-notify` are Go commands.
- **`captaincore progress [--clean]`** shows running bulk operations (PID, percent, elapsed, parallelism). **`captaincore task list [--limit=] [--fleet]` / `task get <id>`** read the server task database and mark stalled processes. **`captaincore monitor stats [--limit=20]`** summarizes recent monitor runs.
- **`captaincore script list`** lists the built-in remote scripts with descriptions parsed from their headers.

**Logs, email, updates, misc**

- **`captaincore logs list <site>`, `logs get <site> --file=<name> [--limit=N]`** with Kinsta and Rocket.net log discovery.
- **`captaincore logs archive <site|@target> [--dry-run] [--skip-if-recent=24h] [--parallel=5]`** streams rotated access and error logs to B2 under `{rclone_backup}/{site}_{id}/{env}/logs/`, using B2 as the source of truth for what is already archived. **`logs archive-list <site>`** and **`logs archive-get <site> <file> [--expire=24]`** (signed URL) expose the archive for forensics. Matching REST endpoints live in captaincore-manager.
- **`captaincore email-health send|generate|response <site|target>`** for deliverability testing.
- **`captaincore update-log get|generate|list|list-generate <site>`** for WordPress update history.
- **`captaincore performance-monitor activate|deactivate|fetch <site> [--hours=N]`.**
- **Bulk core-update probe.** `captaincore ssh @target --script=update-core` prints one line per site, stores the full per-site run (core before/after, stage, excerpt) on the Manager, and emails a grouped failure recap.
- **`captaincore archive list` / `archive share <file>`** (7-day public link), **`snapshot add` / `snapshot list`**, **`account-portal sync|delete`**, **`monitor-notify <account-portal-id>`**, **`default-sync`**, **`manifest-generate`** (site counts, storage and quicksave metrics from SQLite), **`regenerate-thumbnails <site>`** (800px and 100px from capture screenshots), **`account delete`**.
- **Standby mode** (`captaincore_standby`) pauses automated operations.
- **`captaincore server`:** `POST /run/stream` streams command output over HTTP, `GET /task/{id}/stream` streams task status as SSE, `GET/PUT/DELETE /task/{id}` manage tasks, `/progress` exposes bulk-run progress. `CAPTAINCORE_SERVER_BIND` pins the bind address (default `:8000`). The splash page carries the current CaptainCore brand as embedded webp assets.

**Remote scripts (38 new)**

- **Security detection:** `malware-hunt` (eight-section scanner with a verified-component exclude list), `db-code-audit` (executable code stored in options, snippets and widgets), `detect-database-triggers`, `detect-binary-payloads`, `detect-web3-injection` (EtherHiding and service-worker loaders), `detect-seo-spam`, `detect-seo-cloaking`, `detect-upload-probes`, `detect-elevated-permissions`, `detect-fake-dates` (timestomped PHP), `detect-forged-registrations`, `detect-malformed-passwords`, `detect-user-enumeration`, `report-compromised-passwords`, `component-hashes`, `plugin-diff` (against a clean wordpress.org copy), `check-security-log-size`.
- **Maintenance:** `update-core` (sideload a core build, boot the live site against it, probe for fatals, then `--apply`; `--version=latest|nightly|next|x.y.z`, `--probe-only`), `detect-empty-homepage`, `prepare-wordpress`, `php-in-uploads`, `vault` (restic-over-B2 full-site snapshots: `create|snapshots|snapshot-info|mount|info|prune|delete`), `restic-cache-check` / `restic-cache-purge`, `performance-monitor-deploy` / `-remove`, `file-manager` (home-jailed list, view and delete behind the Manager's Files tab).
- **Data collection:** `fetch-folder-size`, `fetch-database-tables`, `fetch-error-log-size`, `fetch-log-file`, `fetch-log-files`, `fetch-users-logins`, `archive-logs`, `quicksave-fingerprint`, `email-health-check`, `arguments`.
- **Signature data:** `lib/malware-signatures.json`, `lib/capture-signatures.json`, `lib/capture-plugin-pages.json`.

### Improvements

- **`fetch-site-data`** now reports `mu_plugins`, `core_verify_checksums` with details, `plugin_checksum_details`, `php_version`, `php_memory`, `default_role`, `registration`, `db_size`, `error_logs`, `session_signal`, `component_hashes`, `mu_plugin_files`, `core_file_hashes`, `loose_file_hashes` and `capture_plugin_pages`. Credentials come from `wp config get` instead of grepping `wp-config.php`. The session signal is capability-based and never loads all users.
- **CaptainCore Helper 0.7.3** (deployed by `deploy-helper`): magic login, a security log with a `security-log` WP-CLI namespace, user-enumeration protection, a security-patch updater, and password-breach blocking via the HIBP k-anonymity API.
- **`deploy-mailgun` now installs Gravity SMTP** (generic SMTP primary, PHP mail backup) and requires `--name=`.
- **`captaincore ssh`** parses its own flags so unknown flags pass straight to the remote script; single-site runs are native Go and `@targets` fan out. **Bulk targeting** everywhere: `@all`, `@production`, `@staging` with `.monitor-on`, `.updates-on`, `.updates-off`, `.offload-on`, `.offload-off`, `.backup-local`, `.backup-remote` suffixes, `--parallel=N`, `--label`, `--fleet`, and per-run progress files for `captaincore progress`. `--skip-if-recent` and `--dry-run` are on `backup generate`, `quicksave generate`, `quicksave backup`, `update` and `logs archive`.
- **Updates:** `captaincore update <site|@target> [--parallel=5] [--skip-if-recent=24h] [--dry-run]` takes a quicksave before and after and writes an update log only when the hash changed, prints `Updated <name> <old> -> <new>` lines, surfaces the first stderr errors when stdout is empty, and emails the admin when the plugin count changes across the run. The remote `update` script toggles maintenance mode, runs a timed second plugin pass after a cache flush, handles Elementor, Elementor Pro, AdRotate Pro and WooCommerce database upgrades (network-aware), flushes Elementor CSS and purges the Kinsta cache.
- **Monitor** toggle per environment.
- **Captures:** `capture generate` retries the screenshot fetch five times with backoff and validates the response before accepting it, supports ImageMagick 7 (`magick`) with a `convert` fallback, and tracks checkout, cart and account pages (`lib/capture-plugin-pages.json`) for injected-script diffs without screenshotting them, then runs `capture check` and `sync-data`.
- **Site data:** `site list` / `site get` gained `--format=json`, `--field` and `--bash`; `site sync --update-extras`; `deploy-defaults --global-only`; `screenshot --parallel`; `sync-data --parallel --json --skip-screenshot`; `stats-deploy --parallel`.
- **Backup excludes** add `node_modules/`, `/error_log`, `/tmp` and the cache and backup directories of ai1wm, Duplicator, BackupWordPress, Akeeba, BackupBuddy, WP Super Cache and Duplicator.
- **Loose-file scanning** uses a single SSH plus tar pipe instead of one `scp` per file (sites with vendored libraries went from 14 hours to minutes).
- **`db-backup`** reads credentials with `wp config get`, accepts `../tmp` as a private directory, and falls back cleanly when `--single-transaction` or `--no-tablespaces` is refused. **`migrate`** URL-decodes the source, rewrites Dropbox links, migrates `mu-plugins`.
- **`deactivate <site>`** returns HTTP 503, skips admin and WP-CLI, and takes `--name`, `--link`, `--subject`, `--status`, `--action`; paired with `activate`.
- **`plugins-zip`** accepts a comma-separated slug list and purges the Kinsta CDN so cookbook installs get the fresh zip.
- **Stats:** official Fathom API replaces Fathom Lite.
- **Server:** tasks carry an argv array (`Args`) and are executed with `exec.Command` instead of a concatenated shell string (the legacy inline `--payload` form still parses); the `token` header is compared in constant time; `/progress` routes are scoped to the calling tenant and validate the PID; `/task/{id}/stream` streams status as server-sent events; websocket disconnects clean up their client and a failed upgrade no longer kills the daemon; the line scanner buffer is 8 MB so long file-manager lines do not stall a request; `ReadTimeout`/`WriteTimeout` are unset so long streams are not cut off; and the parsed config (which holds API tokens) is no longer printed on start.
- **Security hardening.** `config.json` and the SQLite files are forced to `0600`. Fields reported by a site (address, username, port, home directory, home URL, and now the site slug and SSH key name) are validated before they reach a shell command or a filesystem path; `connect` drops a synced site whose slug is not a plain identifier, and `ssh` refuses an unsafe slug or key name instead of building the command. Values passed into the remote environment are escaped for both shell layers, so `$(...)` and backticks in a site-reported value no longer run on the CaptainCore server before the ssh payload is sent.
- **CLI server hardening.** Caller-supplied `--captain-id`, `--fleet` and `--config` are stripped from task arguments before the server prepends its own tenant scope, so a task can no longer re-scope itself onto another tenant or fan out across all of them. Updating a task that belongs to another tenant returns 404 instead of inserting a new row. The token-to-tenant lookup uses a constant-time compare. Websocket connections are held to the server's own origin or `CAPTAINCORE_SERVER_ORIGINS`. Task tokens are no longer written to the service journal, and a blank progress log line no longer panics the handler.
- **Bash `declare` guard.** Every `declare` in the operations scripts and remote scripts checks that the variable name is a plain shell identifier first, since bash evaluates an array subscript arithmetically and would run a command substitution hidden in a crafted name. `lib/arguments` gains an `is_identifier` helper.
- **Release tag validation.** `captaincore upgrade --version=` and the tag read from the GitHub redirect must look like a release tag before they become part of a download URL.
- **`update-core` probe harness:** empty and non-WordPress roots skip instead of failing, the loopback falls back to `127.0.0.1`, page HTML mentioning "fatal error" is not treated as a PHP fatal, and probe memory is raised to 512M.

### Bug Fixes

- `backup download` failed when the payload came as `--payload` instead of a positional argument.
- `backup prune` and `backup migrate-v2` could collide with a running backup.
- `site sync --update-extras` called the wrong capture path.
- Recipe `--code` was decoded through a double-quoted bash string, which ate backslash-newline continuations and expanded `$vars`; it is now written to disk byte for byte.
- `stats-deploy` did not fan out in bulk mode; recursive `bulk` invocations are blocked.
- Fixed archive share URL generation, numerous PHP 8 warnings, sync and site-sync reliability, bulk usage calculations, WordPress.com stats, email-health concurrency, `xargs` cross-platform compatibility, quicksave backup and directory restore, SSH argument passing and default key selection, an SCP transfer bug, site selection by `site_id`, captain ID handling, Rocket.net staging sites, large and concurrent syncs, `core_verify_checksums` return handling, staging and production deploy defaults, bulk command execution, drift `--hashes` flag binding, and a timeout in long-running operations.

### Removed

- The private WordPress install and WP-CLI under `data/` as the CLI's data store.
- 45 PHP scripts from `lib/local-scripts/` (six helpers remain and no longer need WordPress).
- 57 files from `app/`, replaced by Go commands in `cmd/` (which grew from 29 to 59 files): the `cli/`, `captain/`, `dns/`, `key/`, `get/` and `utils/` trees, `monitor`, `monitor-check`, `ssh`, `ssh-detect`, `sync-data`, `size`, `copy`, `configs`, `scan-errors`, `regenerate-thumbnails`, `manifest-generate`, `update-fetch`, `usage-update` and the `*-generate` / `*-list` bash variants (the `backup get-generate`, `backup list-generate`, `quicksave get-generate` and `quicksave list-generate` commands still exist, now in Go).
- The `_do` remote helper, which was added and retired inside this release window; its snapshot half lives on as `vault`.
- The old PNG logo set on the server splash page.

## [0.13.0] - 2021-11-20
### Added
- Commands `quicksave list`, `quicksave list-generate`, `quicksave list-missing`, `quicksave get` and `quicksave get-generate` which power the improved Quicksave GUI.
- Command `backup list-missing` to generate missing backup responses for GUI.

### Changed
- Command `account sync` has been restored with compatibility fixes for Cobra
- Improved `monitor` server caching handling
- Improved remote script `update` with new WP-CLI `--exec` parameter to quickly pass `define( 'WP_ADMIN', true );` before running updates.
- Massive [quicksave performance improvements](https://captaincore.io/quicksave-performance-improvements/). Generating quicksaves will now also create individual `commit-<hash>.json` files and a `list.json`. Those cached responses are used to power a very fast GUI.

## [0.12.0] - 2021-07-04
### Added
- Command `server`. Previously was a separate project named CaptainCore Dispatch which has now been merged here.
- Command `configuration get` and `configuration sync`
- Integration for [Fathom Analytics](https://usefathom.com/). Starting phase out support of Fathom Lite.
- Revamped Fathom tracker.js
- Delete accounts

### Changed
- Replaced underlying [Bash CLI](https://github.com/SierraSoftworks/bash-cli) framework with [Cobra](https://cobra.dev/).
- Argument `--captain_id` is now `--captain-id`
- Moved command `backup` to `backup generate` and organized backup related commands to `backup download`, `backup get`, `backup get-generate`, `backup list` and `backup list-generate`.
- Moved command  `quicksave` to `quicksave generate` and organized quicksave related commands to `quicksave file-diff`, `quicksave generate`, `quicksave rollback`, `quicksave show-changes`, `quicksave sync` and `quicksave usage-update`.
- Moved command `site environment` commands to `environment`
- Moved command `snapshot` to `snapshot generate` and organized snapshot related command to `snapshot fetch-link`.
- Moved commands `copy-production-to-staging` and `copy-staging-to-production` to `site copy-to-production` and `site copy-to-staging`.
- Command `site copy-to-staging` now uses a snapshot for improved deployments.

## [0.11.0] - 2020-11-24
### Added
- Command `bulk` to provide standardized way of parallelizing commands.
- Command `site environment list`
- Commands `running get`, `running list` and `running listen`
- Local scripts `process-add.php`, `process-remove.php`, `process-start.php` and `process-track.php`
- Restic infused backups
- Parallelizing to `captaincore backup`
- Global argument `--progress`
- Tracks process ID of parent CaptainCore command. Useful for following progress of bulk background commands.

### Changed
- Remove runner commands `backup-runner`, `ssh-runner` and `sync-data-runner`. Implemented new `bulk` command for running commands in parallel.
- Handles Elementor database upgrades when running `update` script.
- Improved `migrate` script. Better feedback and minor order correction when applying new table prefix.
- Improved `launch` script to use the source domain and properly handle escaped URLs.
- Improved site monitor notifications.
- Command `cli update` now performs CaptainCore database upgrades.
- Store `scan-errors` with site instead of environment.

## [0.10.0] - 2020-09-05
### Added
- Command `scan-errors`
- Command `regenerate-thumbnails`
- Shell environmental variables
- Support for dynamic /wp-content/ location
- Generate high quality home page thumbnails after running `capture`

### Changed
- Site monitor emails notifications now include site names and emojis 🔗 ⌚
- Support custom /wp-content/ path in remote scripts `deploy-fathom` and `deploy-helper`
- Improved remote script `apply-https` to handle more common url replacements.
- CaptainCore Helper v0.2.4: WPS Hide Login plugin compatibility
- CaptainCore Helper v0.2.3: Disable WordPress 5.5 auto-update email notifications for themes and plugins
- CaptainCore Helper v0.2.2: Remove site health widget from dashboard
- CaptainCore Helper v0.2.1: Improves compatibility with custom login plugins
- Replaced Gowitness screenshots with [ScreenshotsCloud](https://screenshots.cloud)

## [0.9.0] - 2020-04-11
### Added
- Command `account sync`
- Command `default-sync`
- Command `site deploy-defaults` as replacement for `site deploy-recipes`, `site deploy-settings` and `site deploy-users`
- Local script `site-deploy-defaults.php` as replacement for `site-fetch-default-recipes.php`, `site-fetch-default-settings.php` and `site-fetch-default-users.php`

### Changed
- Moved quicksave commands under `captaincore site` for better organization
- Improved `update` with support for handling WooCommerce database upgrades
- Improved `site prepare` as replacement for `site deploy-init` and `site deploy-configs`
- Command `site sync` will now trigger global defaults to deploy with argument `--update-extras`
- Renamed command `view-changes` to `show-changes`
- Removed unnecessary commands
- Removed commands `site deploy-init`, `site deploy-configs`, `site deploy-recipes`, `site deploy-settings` and `site deploy-users`. Functionality combined and rolled into `site deploy-defaults` and `site prepare`
- Removed command `login`. No longer needed as same functionality exists with deployed mu-plugin `captaincore-helper.php`
- Removed argument `--skip-deployment` from command `site prepare`

## [0.8.0] - 2020-03-14
### Added
- Remote script `reset-permissions`
- Command `snapshot-fetch-download-link`
- Command `site sync` as replacement for `site add` and `site update`
- Argument `--skip-extras` to `site update`
- File `lib/excludes` which defines list of files to exclude from backups.
- Local script `site-run-updates.php` which handles WordPress updates from `captaincore update`

### Changed
- Command `snapshot` with new data format.
- Improved fleet mode configs within local script `configs.php`.
- Improved `site get`. Will now accept <site-id> in addition to <site-name>.
- Improved `site delete`. Will now be triggered to run in the background.
- Improved `captaincore backup`. WordPress core files are ignored. Database backups now directed to private folder and named `database-backup.sql`.
- Fix bug with remote script `plugins-zip`
- Renamed `captaincore prep` to `captaincore site prepare`
- Renamed `captaincore rclone-configs` to `captaincore site rclone-configs`
- Removed legacy s3 support from `captaincore backup`
- Removed commands `site update-field`, `site add` and `site update`
- Removed local scripts`site-update.php` and `site-update-field.php`

## [0.7.0] - 2020-01-29
### Added
- [CaptainCore WordPress plugin](https://github.com/CaptainCore/captaincore) to CLI's private WordPress data storage.
- Command `captaincore site update-field <site> <field> <value>`
- Command `captaincore captain db-create`
- Argument `--skip-screenshot` to `sync-data` command.
- Argument `--debug` to `screenshot` command.
- Config `rclone_upload` and `rclone_upload_uri`. Defines public bucket to use for capture images.
- Local script `site-update-field.php`
- Local scripts `site-fetch-default-recipes.php`, `site-fetch-default-settings.php` and `site-fetch-default-users.php` as replacement for commands `site fetch-default-recipes`, `site fetch-default-settings` and `site fetch-default-users`

### Changed
- CaptainCore CLI's private WordPress data structure has been significant improved. The CLI now uses the same data structure as the CaptainCore WordPress plugin v0.10.
- Commands `captaincore site list` and `captaincore site get` to work with new data format.
- Argument `--field=ids` changed to `--field=site_id` on `captaincore site list`
- Improved `captaincore update` with built in cache clearing.
- Improved `screenshot` with new gowitness argument `--disable-db`
- Improved local script `configs.php`. Now pulls in keys and values pairs dynamically.
- Fix bug with `screenshot` when folder names contain spaces.
- Updated local scripts `site-get.php`, `site-list.php`, `site-add.php` to use new data storage.
- Command `captaincore capture` run automatically after a new quicksave.

## [0.6.0] - 2019-09-27
### Added
- SSH key management with commands `key add` and `key delete`.
- Command `site deploy-receipes`
- Command `site fetch-default-receipes`
- Arguments`--notes` and `--user_id` to `snapshot` command.
- Argument `--debug` to `monitor-check` command.
- Argument `--debug` to `ssh-runner` command.

### Changed
- SSH connections now hard fail with bad SSH key. Previously they timed out asking for a password.
- Fix bug with argument `--html` with `quicksave-file-diff`
- Improved pulling in site details
- Removed `site fetch-default-plugins` and `site deploy-plugins` as they are no longer needed.

## [0.5.0] - 2019-07-24
### Added
- Command `recipe add`
- Command `run`
- Command `site fetch-settings`
- Command `site deploy-settings`
- Argument `--recipe=<recipe_id>` to `ssh` command.
- Argument `--html` to `quicksave-file-diff` command for safe HTML output.
- Config `path_recipes`

### Changed
- Fix bug where certain password wouldn't add correctly to Rclone.
- Fix bug when running `--fleet`.

## [0.4.5] - 2019-06-29
### Added
- Command `stats-generate`. Integrates with Fathom instance for automatic setup
- Command `manifest-generate`. Generates manifest which tracks CaptainCore usage stats in manifest.json. In fleet mode this is generated per captain.
- Command `quicksave-usage-update`. Generates usage info (count and storage) for quicksaves.
- Local script `manifest-generate.php`
- Config `captaincore_tracker_user` and `captaincore_tracker_pass` for integration with Fathom instance
- Environment support to `stats-deploy` 
- Fleet mode support to `rclone-configs`

### Changed
- Added fleet mode support to `monitor`
- Improved `backup` to use rclone site name with fleet mode support
- Improved local usage stats storage
- Improved argument compatibility with remote script `deploy-fathom` and command `stats-deploy`

## [0.4.4] - 2019-06-04
### Added
- Command `deploy-defaults` for bulk deploying default plugins/users.

### Changed
- Reduced feedback are various commands. This helps keeps command output streaming to the websocket running smoothly.
- Various fixes in `copy-production-to-staging`, `copy-staging-to-production` and script `migrate`. Will properly handle `captain_id`. Fix paths for table_prefix checks. 
- Script `migrate` now automatically selects most recent modified .sql file for import.

## [0.4.3] - 2019-04-22
### Added
- Command `ssh-runner` which adds parallelizing to `ssh`. 
- Config `rclone_screenshots` for automatic upload of screenshots.
- Flag `--skip-remote` to `site deploy-init`.

### Changed
- Replaced argument `--all` with a new flexible targeting argument using the @ symbol. To target use `@all`, `@production` or `@staging`. These can be combined to filter sites further by chaining other modifiers after the target. For example `@production.updates-on` will target production sites are marked for automatic updates and `@all.offload-on`will target all sites which have offload enabled.
- Added an ongoing errors section to site monitor email notifications. 
- Added environment support to `deploy-plugins` and configured plugins to activate. 
- Added gallery and ngg folder to remote script`migrate`.
- Added delay in `monitor` when retrying failed checks. Attempt to reduce false positives.

## [0.4.2] - 2019-04-08
## Added
- Commands `site bare-add`, `site bare-delete` and `site bare-update` to handle site management from other CaptainCore servers. These are used from CaptainCore Dispatch to relay site changes to all servers.
- Argument `--filter` argument to `snapshot` which supports options: database, themes, plugins, uploads and everything-else. 
- Argument `--email-notify` to `backup`. Will no longer send emails by default.

### Changed
- New visits format in `backup`, `copy` and `usage-update`

## [0.4.1] - 2019-03-18
### Added
- Fleet mode  ⛵⛵⛵ which enables CaptainCore to manage sites for mutiple captains (CaptainCore GUIs).
- Global argument `--fleet` which will loop any commands through all captains by `--captain_id=<id>`.
- Config `captaincore_fleet` to activate Fleet mode. Sites will be stored in folders per `captain_id` in format:`<path>/<captain_id>/<site>_<site-id>/`
- Configuration now stored in new `config.json` format.
- Command `screenshot` and `screenshot-runner`. Takes screenshots using https://github.com/sensepost/gowitness and headless Chrome.
- Command `site fetch-token` which replaces `get token` php script.
- Command `site deploy-configs` which replaces `get configs` php script.
- Argument `--direct` to `ssh`
- Argument `--skip-remote` to `quicksave`

### Changed
- Improvements to command `monitor`. New argument `retry` which defines attempts on failures. New data storage `monitor.json` for tracking sites when offline and come back online. New "notify at" times defined as 1 hour, 4 hour and 24 hour. Other failure checks will be ignored per failure.
- Improvement to command `snapshot` performance. 
- Improvement to command `captaincore site deploy-init`. Now deploys helper mu-plugin plugin to both production and staging sites.
- Fixes in remote script `migrate`. Properly handles themes/plugins with spaces in directory names.
- Replaced configrations file `config` for new`config.json`.
- Internally bundled configs script with arguments script. Now script files only needs a single include: `source ${root_path}lib/arguments`.
- Remove global arguments `--run-in-background=<job-id>` and `mark-when-completed`
- Replaced old commands `users`, `users-json`, `plugins-get`, `deploy plugins`, `deploy keys`, `deploy plugins`, `deploy users` and `deploy-init` with completely rewritten, simplified and organized commands `site deploy-init`, `site deploy-keys`, `site deploy-plugins`, `site deploy-users`, `site fetch-default-plugins` and `site fetch-default-users`. Replaced legacy mu-plugin injection method for proper WP-CLI over SSH deployment.

## [0.4.0] - 2019-03-04
### Added
- Environment support to commands `quicksave` and `quicksave-store`
- Added multisite site count to remote script `fetch-site-data`
- Command `captaincore stats-fetch`
- Fetch stats from Fathom

### Changed
- *Breaking changes* - Updated folder structure to include environments. New format: `<path>/<site>_<site-id>/<environment>/`. Each environment can now handle `backups`,`snapshots`, `quicksaves` and `updates`.
- Command `captaincore site get` now returns a single environment (production by default). Example `captaincore site get <site>` and `captaincore site get <site>-production` will return production details whereas `captaincore site get <site>-staging` will return staging.
- Command `captaincore stats` now requires `<site>` instead of domain. This allows stats to be pulled dynamically from various provider (WordPress.com and Fathom).
- Improved error handling with command `captaincore site delete`.
- Remote script `migrate` properly flushes permalinks and ignores SSL errors from source backups.
- Bug fix where certain theme updates were missed due to duplicate json file name.

## [0.3.3] - 2019-02-09
### Added
- Argument `--urls` to `captaincore monitor`

### Changed
- Multisite support to remote script `deploy-fathom`
- Improvements to `captaincore monitor-check`
- Reversed git compare with `quicksave-file-diff`
- Removed argument --skip-uploads from `copy-production-to-staging` and `copy-staging-to-production`. Now always skips uploads and syncs them incrementially using rclone.
- Install matching version of WordPress core when using `copy-production-to-staging` and `copy-staging-to-production`.

## [0.3.2] - 2018-12-31
### Changed
- Bug fix with command `plugins-get`

## [0.3.1] - 2018-12-03
### Added
- Local script `monitor-fetch-errors-and-clean`
- Remote script `deploy-helper`. Deploys a must-use helper plugin for CaptainCore. Initial release include quick login support for CaptainCore GUI.
- Command `captaincore cli backup`. Backups current CaptainCore cli configs to remote.
- Config `rclone_cli_backup` which configures where to store CaptainCore cli backups.
- Argument `--updates-enabled` to `captaincore update`
- Argument `--page` to `captaincore monitor`

### Changed
- Bug fix to resolve [inconsistent arguments with remote scripts over SSH](https://anchor.host/?p=58761).
- Consistent sha-bangs on bash script files.
- Improved local script `monitor-generate-email`
- Reset file permissions.

## [0.3.0] - 2018-10-14
### Added
- Functionality to remote script `migrate`. Files from zip now extract within a new timestamped folder. Supports moving non-default root level files and folders. Supports moving blogs.dir folder for legacy multisite networks. Better feedback while running. Reapplies search privacy settings. Better wp-config.php handling.
- Config `captaincore_branding_slug`. Used when generating stats mu-plugin.
- Config `captaincore_tracker` for running a Fathom Analytics instance. Used when generating stats mu-plugin.

### Changed
- Improved staging and production deploys. Now uses new `migrate` remote script.
- Improvements for `captaincore ssh`. Now checks and fails properly for staging sites which do not exists. Bug fix when sending arguments with spaces to remote script.
- Make sure database export is secured when running `captaincore backup`
- Bug fix causing files within Kinsta cache plugin from being excluded during Quicksaves.
- Bug fix when uploading local zip with `captaincore store-snapshot`
- Bug fix with `captaincore util git-permissions-reset` Only process a max of 1000 files per run.

## [0.2.9] - 2018-09-26
### Added
- Command `captaincore multisite-extract-subsite`. Helps extract subsite from a Multisite network
- Concurrency to `captaincore sync-data`
- Command `captaincore sync-data-runner` for concurrency support

### Changed
- Run `captaincore update` with WP_ADMIN set true for increased compatibility.
- Deactivate `wp-rocket` when using `copy-production-to-staging`

## [0.2.8] - 2018-08-20
### Added
- Argument `<site>` and `<plugin>...` to command `plugins-zip`
- Configs `captaincore_branding_name`, `captaincore_branding_title`, `captaincore_branding_author`, `captaincore_branding_author_uri`, `captaincore_server`, `WPCOM_API_KEY`, `GF_LICENSE_KEY` and `ACF_PRO_KEY` to `config` file

### Changed
- Command `captaincore plugins-zip` now handled through single SSH request.
- Command `captaincore cli update` now pulls via git.
- Command `captaincore backup` now using rclone link sharing rather then separate script.
- When doing a `wp search-replace` always use argument `--report-changed-only`
- Revised `readme.md` and `config.sample` documentation.
- Renamed directory `lib/php` to `lib/local-scripts` for better clarity and consistency.
- Renamed remote scripts for applying https to `apply-https` and `apply-https-with-www`.
- Renamed `captaincore utils localwp` to `captaincore local-create-wordpress`
- Renamed `captaincore utils import-prod-db-locally` to `captaincore local-db-import-from-production`
- Bug fix with `captaincore upload`.
- Bug fix remote script `fetch-site-data` when trimming whitespace.
- Improved `readme.md`
- Improved `captaincore get transferred-stats` regex matching. Accepts `<file>` argument directly rather then `--file=<file>`.
- Improved `curl` requests. Now uses defined config `$captaincore_gui`.

### Removed
- Argument `--file` from `captaincore get transferred-stats`
- Unused submodule dnsrecon
- Dropbox uploader script

## [0.2.7] - 2018-08-06
### Added
- Command `captaincore monitor-check <site>`. Standalone script which allows checking of individual valid urls for parallelizing purposes.
- Local script `monitor-error-count`
- Local script `monitor-generator-email`
- [Emoji-Log](https://github.com/ahmadawais/Emoji-Log) for git commits.

### Changed
- Improved remote script `fetch-site-data`. Results are now striped of whitespace.
- Run `sync-data` during site prep
- After `quicksave` backup quicksave to remote
- Improvements to `captaincore monitor`. Added basic email reporting for errors and warnings. Add max time of 30 secs per check. Parallel now defaults to 20. Refactored code. Moved email generation and error count to PHP.

## [0.2.6] - 2018-07-15
### Added
- Command `captaincore update-logs-store <site>`
- Command `captaincore quicksave-store <site>`
- Argument `--all` to `captaincore ssh`
- Remote script `rewrite-prep`

### Changed
- CaptainCore API moved to custom WordPress rest endpoint. All communication to API now require a `site_id`.
- Renamed remote scripts `applyssl` and `applysslwithwww` to `applyhttps` and `applyhttpswithwww`
- Renamed config `captaincore_wordpress_site` to `captaincore_gui`
- Remote script `migrate` - Only download if local file doesn't exist.
- Command `captaincore update` now send update logs to CaptainCore API.

### Removed
- Argument `<git_hash_previous>` from `captaincore quicksave-file-diff`. This is now automatically calculated.
- Domain requirement from CaptainCore API. Will need determine site from included `site_id`.

## [0.2.5] - 2018-07-01
### Added
- Global argument `--run-in-background=<job-id>`
- Command `captaincore job-fetch <job-id>`
- Command `captaincore login <site> <login> [--open]`
- Argument `--updates-enabled` to `captaincore site list`
- Remote script `fetch-site-data`

### Changed
- Improved json output of `captaincore update-fetch`
- Quicksaves now collects user data
- Improvements to `captaincore sync-data`. Added `--all` argument.
- Improvements to `--run-in-background` json output.
- Bug Fix: Curl argument list too long. All curl requests to CaptanCore API now use JSON format via standard input.

## [0.2.4] - 2018-06-17
### Added
- Command `captaincore open`. Opens one or more sites in browser.
- Command `captaincore get config-from-api --field=<field>` for fetching dynamic info from GUI.
- Configs `captaincore_admin_email` and `rclone_logs` to `config` file
- Arguments `[--exclude-themes=<theme-names>]` and `[--exclude-plugins=<plugin-names>]` to `captaincore update`

### Changed
- *Breaking changes* - Updated folder structure to include site IDs. New format: `<path>/<site>_<site-id>/`. Updated remote path to match local subfolder format `/backup`. Using a unique site ID allows sites to share the same name from different host providers.
- Moved all action commands, not relating to site configs, from `captaincore site` to top level `captaincore`. Those include `activate`, `deactivate`, `prep`, `rclone-configs` and `sync-data`.
- `captaincore deploy plugins` now pulls standard set of plugins via WordPress API
- `captaincore quicksave` now sends git commit timestamp to GUI
- Improved docs and added `--all` to `captaincore monitor`
- Moved command `get quicksave_file_diff` to `quicksave-file-diff`
- Moved command `get quicksave_changes` to `quicksave-view-changes`
- Moved command `deploy production-to-staging` to `copy-production-to-staging`
- Moved command `deploy staging-to-production` to `copy-staging-to-production`
- Moved command `get transferred_stats` to `get transferred-stats`
- Moved command `utils store-snapshot` to `store-snapshot`

### Removed
- Duplicate command `quicksave_status`
- Commands `get log_stat`, `get log_stats` and `get server`
- Command/library dnsrecon

## [0.2.3] - 2018-06-03
### Added
- Command `captaincore size`. Calculates size of one or more sites.
- Argument `--file=<file>` to `captaincore rollback`. Handles rollback of individual Quicksave file.
- Arguments `[--field=<field>]`and `[--search-field=<field>]` to `captaincore site search`

### Changed
- Updated `<site>` format to support a host provider `<site>@<provider>`. The classic `<site>` will continue to work however won't be very particular if multiple site names exist. This should make dealing with multiple host providers enjoyable. Here's an example coping a site between providers `captaincore copy anchorhost@wpengine anchorhost@kinsta`.
- Support for new provider field when running `captaincore site add` or `captaincore site update`
- Improvements for `captaincore site get`. Moved complex logic into PHP needed for supporting new `<site>` with providers.
- Improvements to Readme
- Improvements to `captaincore site search`. Will now return partial domain and address matches.
- Fixed name output in debug message.
- MacOS compatibility fix for `captaincore quicksave`

### Removed
- Housecleaning commands.

## [0.2.2] - 2018-05-20
### Added
- Global argument `--mark-when-completed` which adds json output after command finishes. Example: `{"response":"Command finshed","timestamp":"2018-05-09-213121","runtime":"5"}`. Used to track background jobs initiated from CaptainCore GUI.
- Command `captaincore copy <site-source> <site-destination> [--email=<email>]`
- Arguments `--name` and `--link` to `captaincore site deactivate` for custom links on deactivated sites.

### Changed
- Improvements to `captaincore ssh`. When using a `--script` automatically pass the current site via `--site` reducing the need to manually pass that info along.
- Improved display of deactivated sites.
- Better output with remote script `launch`
- Require arguments `--site` and `--domain` on remote script `launch`
- Improved output of remote scripts `applyssl`, `applysslwithwww` and `launch` by only reporting changes.

### Removed
- Argument `--site` from remote script `launch`. This is now handled automatically.

## [0.2.1] - 2018-05-08
### Added
- Command `captaincore update` for themes/plugin updates. Changes are logged in json files.
- Command `captaincore update-fetch` to return update logs in json format
- Command `db-restore` for pulling in a revision from Rclone remote.
- Argument `[--all]` to `captaincore backup` for backing up all sites
- Argument `[--all]` to `captaincore update` for updating all sites
- Argument `[--all]` to `captaincore quicksave` for quicksaving all sites
- Arguments `[--all]` and `[--skip-backup]` to `captaincore snapshot`
- Require `<site>` for `captaincore site get`
- Require `<site>` and `<commit>` for `captaincore rollback`
- Output `site_id` to `captaincore site get`.
- Script `lib/arguments` to handle bash arguments removing duplication
- Automatic removal of files from remote storage when removing sites.
- File permission reset to `captaincore cli update`

### Changed
- *Breaking changes* - Switched folder structure from domains to site names. Consolidated folders under sites rather then separate top level organization for backups and quicksaves. New format: "<path>/<site>/backup", "<path>/<site>/quicksave" and "<path>/<site>/updates"
- Moved functionality of `captaincore generate snapshots` into `captaincore snapshot`
- Moved `users` and `users-json` commands to root level
- Quicksave will no longer happen as part of the backup process
- Improved Quicksave functionality to run standalone without a full backup requirement.
- Fix for uploading snapshots to remote storage
- Fixed staging/production deployment emails
- Improved docs for `captaincore backup`, `captaincore rollback`, `captaincore update` and `captaincore site get`
- Improved sample config file
- Replaced `install(s)` to `site(s)` throughout docs and code
- Replaced `[--skip-url-override with]` with `[--update-urls]` in migrate script. Default behavior now keeps sources urls when migrating sites.
- Excluded certain files when unzipping during migrations
- Generalized ssh script `update`. Will now pass through any arguments to `wp plugin update` and `wp theme update`.
- Revised definable `$path`. It's now used by `backup`, `quicksave` and `update` commands.
- Renamed `lib/ssh/` to `lib/remote-scripts`

### Removed
- Bundled bash cli command `captaincore cli uninstall`
- Command `captaincore get stats`
- Command `captaincore generate snapshots`

## [0.2.0] - 2018-04-22
### Added
- Command `captaincore cli update`
- Support for flags with special characters with `captaincore ssh`

### Changed
- Renamed deployment commands to `captaincore deploy production-to-staging` `captaincore deploy staging-to-production`
- Renamed lib ssh_scripts folder to ssh
- Renamed email command from `dns email-lookup` to `dns email`
- Renamed commands `dns bulk-domain` and `dns bulk-nameserver` to `dns domain` and `dns nameserver`
- Renamed `captaincore generate plugins_zipped` to `captaincore generate plugins-zipped`
- Renamed <install> to <site> throughout usage documentation
- Renamed command `captaincore generate usage` to `captaincore usage-update`
- Moved and renamed command `captaincore get backup_status` to `captaincore backup-status`
- Moved and renamed command `captaincore generate plugins` to `captaincore plugins-get`
- Moved and renamed command `captaincore generate plugins-zipped` to `captaincore plugins-zip`
- Moved and renamed command `captaincore generate rclone` to `captaincore site rclone-configs`
- Moved command `captaincore generate localwp` to `captaincore utils localwp`
- Moved command `captaincore generate quicksave` to `captaincore quicksave`
- Deactivate 'login-recaptcha' plugin on staging deployment
- Cleaned up whitespace and added comments to SSL scripts
- Improvements to `migrate` script. Update table prefix if changed. Expand database search.

## [0.1.8] - 2018-04-08
### Added
- Argument `[--skip-uploads]` to `captaincore deploy production_to_staging_kinsta`
- Command `captaincore dns ips-from-site-names <site> [<site>] [<site>] [--skip-follow]`
- Command `captaincore dns email-lookup <site> [<site>] [<site>]...`

### Changed
- Updated `config.sample`. Remove trailing slash as it will cause issues with backups.
- Improvements to `captaincore ssh [<site>] --script=db-convert-to-innodb`. Will count MyISAM tables and convert to InnoDB if needed.
- Renamed underscores to dashes with `captaincore dns bulk-domain` and `captaincore dns bulk-nameserver`
- Fix for plugin deploys and plugin rollbacks
- Standardized arguments for migration script `captaincore ssh <site> --script=migrate --url=<backup-url> [--skip-url-override]`
- Clean up and added comments to migrate script

## [0.1.7] - 2018-03-25
### Added
- Deployable scripts `verify-google-analytics-takeover --verifycode=<verifycode> --uacode=<uacode> --email=<email>`, `launch --install=<install> --domain=<domain>` and `update`
- Argument `[--skip-url-override]` to deployable script `migrate <backup-url>`
- File support to `captaincore utils store-snapshot <url|file>`
- Configurable variables rclone_archive, rclone_backup and rclone_snapshot to config file to manage Rclone remote locations.

### Changed
- Fixed various typos in comments
- Argument support to deployable ssh scripts
- Renamed command store_snapshot to store-snapshot
- Improvements to `captaincore ssh --script` to allow for passing arguments. For example `captaincore ssh <site> --script=<script> --<arg1>=arg1 --<arg2>=<arg2>`
- Improvements to migrate script. Wget progress now displays properly. Fix for db import with buggy plugins/themes.
- Exclude zip files from Quicksaves

## [0.1.6] - 2018-03-18
### Added
- Subcommand `captaincore site search <search>` to find sites by domain
- Argument `--all` to `captaincore rollback <site> <commit>` to rollback entire quicksave
- Arguments `[--filter=<theme|plugin|core>]` `[--filter-slug=<slug>]` `[--filter-version=<version>]` `[--filter-status=<active|inactive|dropin|must-use>]` to `captaincore site list`
- File to remove duplicate quicksaves with `wp eval-file remove-duplicate-quicksaves.php`
- Added installations steps to generate local WordPress site to `captaincore cli install`
- Usage info for rollback command `captaincore rollback --help`
- Argument `[--script-args=<script-args>]` to `captaincore ssh --script`. Example: `captaincore ssh <site> --script=migrate --script-args=<backup-url>`
- Collection of scripts (applyssl, applysslwithwww, db-import, migrate-to-kinsta) to be used with `captaincore ssh --script`.
- Argument `--field=ids` to `captaincore site list`
- Command `captaincore utils sync-with-master` to keep captaincore cli running locally in sync with master. To config add `captaincore_master` and `captaincore_master_port` vars to ~/.captaincore/config file.
- Argument `[<site-id>]` to `captaincore utils sync-with-master` which will force sync a particular site by id

### Changed
- Deploy keys and generate token on `captaincore site update`.
- Improvements to `captaincore utils store_snapshot`
- If script doesn't exist attempt running locally. `captaincore ssh --script`
- Updated usage info for ssh command `captaincore ssh --help`
- Command `captaincore ssh` will attempt to retrieve unknown sites by triggering `captaincore utils sync-with-master`
- Significant improvements to `migrate` script in order to work on both WP Engine and Kinsta.

### Removed
- Arguments `[--plugin]` `[--theme]` `[--plugin-status=<plugin-status>]` and `[--theme-status=<theme-status>]` have been removed and functionality moved to new filter arguments on `captaincore site list`

## [0.1.5] - 2018-03-04
### Added
- Arguments `--command=<command>` and `--script=<file>` to `captaincore ssh`
- Argument `--bash` to `captaincore site get <install>` which allows bash to read credentials stored in CLI's own private WordPress storage.
- New method for loading site credentials into bash
- Command `captaincore site sync-data [<install>]`
- Command `captaincore monitor <site>`
- Argument `[--parallel=<number-of-checks>]` to `captaincore monitor` which controls how many health checks are run at the same time

### Changed
- Site credentials are now stored in private WordPress site within CaptainCore CLI.  
- Switched internal commands to use new ssh argument `--command=<command>`
- Moved site functions from `config` command under `site` command
- Renamed site command `new` to `add` for better consistency  
- Major rework on all `captaincore site` commands to use new WordPress storage.
- Added argument `--field=<field>` to `captaincore site list`
- Fixed regex bug with ssh argument `--command`
- Fixed backups for sites not running WordPress
- Support for ftp sites backups only

### Removed
- Command `captaincore ssh-batch` and merged functionality into `captaincore ssh`
- Default docs for `captaincore cli install`
- Command `captaincore site process` as it's no longer needed since switching over credential storage to WordPress.
- Local text based `logins` file which previously was storing site credentials.
- Command `captaincore get domains` as functionality has been merged with `captaincore site list --field=domain`

## [0.1.4] - 2018-02-25
### Added
- Quicksave file diff command `captaincore get quicksave_file_diff <install> <git_hash_current> <git_hash_previous> <file>`
- New command `captaincore deploy staging_to_production_kinsta <install> --email=<email>`
- New command `captaincore ssh-batch <command>`
- Argument `--email=<email>` to `captaincore deploy production_to_staging_kinsta`

### Changed
- `captaincore deploy keys <install>` now deploys to Kinsta staging sites
- Major performance increases to `captaincore deploy production_to_staging_kinsta`. Switch over to zip/extract wp-content folder instead of sftp sync.
- Standardized site address PHP variable. All $ipAddress are now $address.

## [0.1.3] - 2018-02-18
### Added
- Rollback command `captaincore rollback <install> <commit> [--plugin=<plugin>] [--theme=<theme>]`
- Argument `--force` to `captaincore generate quicksave` to manually force add even if no changes were made
- Argument `--public` to `captaincore upload <install>` which is used for the new rollback command
- Get quicksave status command `captaincore get quicksave_status <install> <git_hash>`
- URL encoding to curl commands
- Added Kinsta staging support to `captaincore upload`

### Changed
- Excluded unnecessary files for quicksave `*.log, *.log.txt and cache/`
- Patch to work around WPE SSH WP-CLI username bug
- Curl now posts to CaptainCore API
- Updated header info for `captaincore`

## [0.1.2] - 2018-02-11
### Added
- Deploy ssh keys `captaincore deploy keys <install>` which is needed before using SSH/WP CLI on Kinsta sites.
- Generate plugin zips for easy deployment `captaincore generate plugins_zipped`
- Quicksave command `captaincore generate quicksave <install>` which captures nightly version numbers for plugins, themes and core.
- Get quicksave changes command `captaincore get quicksave_changes <install> <git_hash>`

### Changed
- Simplified internals for `captaincore config new`
- Upgraded deploy users to new format
- Rewrote `captaincore deploy plugins` to use SSH/WP-CLI
- Deploy token using Anchor API
- Fix for stats during nightly backup
- Changed backup snapshots to use Zip format because non geeks don't know what tar files are

### Removed
- Deploy to backup API

## [0.1.1] - 2018-02-05
### Added
- Argument `--delete-after-snapshot` to `captaincore snapshot`
- Config process command `captaincore config process` which will safely update the logins only when needed
- Generate rclone command `captaincore generate rclone <install>` which now imports staging credentials automatically.
- Setup instructions to readme.md for installing rclone systemwide
- Generate usage command `captaincore generate usage`
- Command for grabbing Quicksave changes from git repo

### Changed
- Load bash config file dynamically
- Upgraded `captaincore config update` command to new format
- Switch `captaincore config update` and `captaincore backup` to use new generate rclone command
- Delete command format is now `captaincore config delete --install=<install> --domain=<domain>`
- Only make snapshot if domain for install is found

### Removed
- Unnecessary delete.sh support file
- Bash variable $path_rclone

## [0.1.0] - 2018-02-02
### Added
- Bulk domain lookup command `captaincore dns domain <domain>`
- Bulk nameserver lookup command `captaincore dns nameserver <domain>`
- File upload command `captaincore upload <install> <local-file>`
- Config get command `captaincore config get <install> --field=<field>`
- Local WordPress generator (MacOS + Valet required) `captaincore generate localwp <folder>`
- [DNSRecon](https://tools.kali.org/information-gathering/dnsrecon) `captaincore utils dnsrecon <domain>` see `captaincore help utils dnsrecon` for setup configuration
- CLI usage and help documentation
- Argument to snapshot `--skip-remote`
- Tools for resetting file permissions within local git repo
- Database backup and `--skip-db` to `captaincore backup [<install>]`

### Changed
- Renamed project to [CaptainCore CLI](https://captaincore.io/)
- Migrated to structured CLI using [Bash CLI](https://github.com/SierraSoftworks/bash-cli). See `captaincore help` for getting started.
- Implemented Rclone v1.39 new `rclone config create` for adding/removing sites
- Moved WordPress.com API to config file

### Removed
- Old structure `~/Scripts/{Action}/{Task}.sh {installname}`
- Lftp dependencies and replaced with Rclone
- Unnecessary .php and .sh extensions

## [0.0.1] - 2017-06-19
### Added
- Initial release
