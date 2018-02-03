# Changelog

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
