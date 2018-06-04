# CaptainCore CLI
[![Logo](https://captaincore.io/wp-content/uploads/2018/02/main-web-icons-captain.png)](https://captaincore.io)

## Description
[CaptainCore](https://captaincore.io) is a powerful toolkit for managing WordPress sites via SSH & WP-CLI. Supports hosting providers [WP Engine](https://wpengine.com/) and [Kinsta](https://kinsta.com/). Powers the CaptainCore GUI (WordPress plugin). Built using [Bash CLI](https://github.com/SierraSoftworks/bash-cli).

## Getting started

- Install `captaincore` command by running `sudo ln -s ~/.captaincore-cli/cli /usr/local/bin/captaincore`
- Download latest rclone and install system wide by running `sudo ln -s ~/Download/rclone-v1.41-osx-amd64/rclone /usr/local/bin/rclone`
- Copy config.sample.sh to config and configure with appropriate folders
- Run `captaincore utils dropbox_uploader` and config with Dropbox account
- Run `rclone config` and config Dropbox account
- Install WP-CLI `curl -O https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar; chmod +x wp-cli.phar; sudo mv wp-cli.phar /usr/local/bin/wp`
- Install JSON package `sudo npm install --global json`
- Install NGINX `sudo apt-get install nginx && sudo systemctl enable nginx.service && sudo systemctl start nginx.service`
- Install MariaDB `sudo apt-get install mariadb-server mariadb-client && sudo systemctl enable mysql.service && sudo systemctl start mysql.service && sudo mysql_secure_installation`
- Copy MariaDB root password to `~/.captaincore-cli/config` as `local_wp_db_pw="<db-password>"`


## Usage

### How site names work

CaptainCore using arbitrary site names. When managing multiple sites, there needs to be a way to uniquely identify each site. While domain names seems like a good option, there are times you may want to be managing the same site with the same domain on multiple host providers. Also domain names can be long and sometimes change. On of flip side using a completely arbitrary site ID isn't very human friendly. A site name is something in between. A short but meaningful name that is unchangeable even if the domain name changes.

Site names can also specify a provider using an @ symbol `<site>@<provider>`. This makes dealing with multiple host providers enjoyable. Here's an example coping a site between providers `captaincore copy anchorhost@wpengine anchorhost@kinsta`. Omitting the provider is completely valid however won't be very particular if multiple site names exist.

### Commands

Shows help
`captaincore help`

Adds a site to CaptainCore CLI.
`captaincore site add <site> --id=<id> --domain=<domain> --username=<username> --password=<password> --address=<address> --protocol=<protocol> --port=<port> --staging_username=<staging_username> --staging_password=<staging_password> --staging_address=<staging_address> --staging_protocol=<staging_protocol> --staging_port=<staging_port> [--preloadusers=<preloadusers>] [--homedir=<homedir>] [--s3accesskey=<s3accesskey>] [--s3secretkey=<s3secretkey>] [--s3bucket=<s3bucket>] [--s3path=<s3path>]`

Removes a site from CaptainCore CLI.
`captaincore site delete <site>`

Backups one or more sites.
`captaincore backup [<site>...] [--all] [--use-direct] [--skip-remote] [--skip-db] [--with-staging]`

SSH wrapper
`captaincore ssh <site> [--command=<commands>] [--script=<name|file>] [--<script-argument-name>=<script-argument-value>]`

Snapshots one or more sites.
`captaincore snapshot [<site>...] [--all] [--email=<email>] [--skip-remote] [--delete-after-snapshot]`

Shows last 12 months of stats from WordPress.com API.
`captaincore stats <site>`

## License
This is free software under the terms of MIT the license (check the LICENSE file included in this package).
