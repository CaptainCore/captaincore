# CaptainCore CLI

## Description
Bulk management tools for WordPress websites. Supports hosting providers [WP Engine](https://wpengine.com/) and [Kinsta](https://kinsta.com/). Powers the CaptainCore Hosting Dashboard (WordPress plugin).Built using [Bash CLI](https://github.com/SierraSoftworks/bash-cli).

## Website
https://captaincore.io

## Getting started

- Install `captaincore` command by running `sudo ln -s ~/.captaincore-cli/cli /usr/local/bin/captaincore`
- Run `captaincore utils dropbox_uploader` and config with Dropbox account
- Copy config.sample.sh to config and configure with appropriate folders
- Run `rclone config` and config Dropbox account

## Usage

	# Shows help
	`captaincore help`

	# Adds website
	`captaincore config new --install=anchorhost --domain=anchor.host --username=anchorhost --password=FtGHRsoxfNg8uEcrMEcVvT --address=anchorhost.kinsta.com --protocol=sftp --port=3289 --preloadusers=2823 --homedir=/www/anchorhost_243/public`

	# Removes website
	`captaincore config delete anchorhosting anchor.host`

	# Backup website
	`captaincore backup <install>`

	# SSH into website
	`captaincore ssh <install>`

	# Generate backup snapshot
	`captaincore snapshot <install> --email=<email>`

	# Get stats
	`captaincore stats <domain>`

## License
This is free software under the terms of MIT the license (check the LICENSE file included in this package).
