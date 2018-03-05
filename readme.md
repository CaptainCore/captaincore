# CaptainCore CLI
[![Logo](https://captaincore.io/wp-content/uploads/2018/02/main-web-icons-captain.png)](https://captaincore.io)

## Description
Bulk management toolkit for WordPress sites. Supports hosting providers [WP Engine](https://wpengine.com/) and [Kinsta](https://kinsta.com/). Powers the CaptainCore Server (WordPress plugin). Built using [Bash CLI](https://github.com/SierraSoftworks/bash-cli).

## Website
https://captaincore.io

## Getting started

- Install `captaincore` command by running `sudo ln -s ~/.captaincore-cli/cli /usr/local/bin/captaincore`
- Download latest rclone and install system wide by running `sudo ln -s ~/Download/rclone-v1.39-osx-amd64/rclone /usr/local/bin/rclone`
- Run `captaincore utils dropbox_uploader` and config with Dropbox account
- Copy config.sample.sh to config and configure with appropriate folders
- Run `rclone config` and config Dropbox account
- Install WP-CLI `curl -O https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar; chmod +x wp-cli.phar; sudo mv wp-cli.phar /usr/local/bin/wp`
- Install JSON package `sudo npm install --global json`
- Install NGINX `sudo apt-get install nginx && sudo systemctl enable nginx.service && sudo systemctl start nginx.service`
- Install MariaDB `sudo apt-get install mariadb-server mariadb-client && sudo systemctl enable mysql.service && sudo systemctl start mysql.service && sudo mysql_secure_installation`
- Copy MariaDB root password to `~/.captaincore-cli/config` as `local_wp_db_pw="<db-password>"`

## Usage

Shows help
`captaincore help`

Adds website
`captaincore config new --install=anchorhost --domain=anchor.host --username=anchorhost --password=FtGHRsoxfNg8uEcrMEcVvT --address=anchorhost.kinsta.com --protocol=sftp --port=3289 --preloadusers=2823 --homedir=/www/anchorhost_243/public`

Removes website
`captaincore config delete anchorhosting anchor.host`

Backup website
`captaincore backup <install>`

SSH into website
`captaincore ssh <install>`

Generate backup snapshot
`captaincore snapshot <install> --email=<email>`

Get stats
`captaincore stats <domain>`

## License
This is free software under the terms of MIT the license (check the LICENSE file included in this package).
