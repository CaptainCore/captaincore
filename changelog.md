# Changelog

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
