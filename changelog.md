# Changelog

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
