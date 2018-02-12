# Changelog

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
- Generate rclone commmand `captaincore generate rclone <install>` which now imports staging credentials automatically.
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
