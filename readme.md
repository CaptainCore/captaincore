<h1 align="center">
  <a href="https://captaincore.io"><img src="https://captaincore.io/wp-content/uploads/2018/02/main-web-icons-captain.png" width="70" /></a><br />
CaptainCore CLI 

</h1>

[CaptainCore](https://captaincore.io) is a powerful toolkit for managing WordPress sites via SSH & WP-CLI. Built using [Bash CLI](https://github.com/SierraSoftworks/bash-cli). Integrates with the [CaptainCore GUI (WordPress plugin)](https://github.com/captaincore/captaincore-gui).

[![emoji-log](https://cdn.rawgit.com/ahmadawais/stuff/ca97874/emoji-log/flat.svg)](https://github.com/ahmadawais/Emoji-Log/)

## **Warning**
This project is under active development and **not yet stable**. Things may break without notice. Only proceed if your wanting to spend time on the project. Sign up to receive project update at [captaincore.io](https://captaincore.io/).

## Getting started

Recommend spinning up a fresh VPS running Ubuntu 16.04 with:
- [Digital Ocean](https://www.digitalocean.com/) - great for most people.
- [Backupsy](https://backupsy.com/) - great for cheap storage.

Eventually all of these steps will be wrapped into a single kickstart.sh script. Until then here are the barebones steps to begin.

- Download `git clone https://github.com/captaincore/captaincore-cli.git ~/.captaincore-cli/`
- Install `captaincore` command globally by running `sudo ln -s ~/.captaincore-cli/cli /usr/local/bin/captaincore`
- Download latest rclone and install system wide by running `sudo ln -s ~/Download/rclone-v1.42-linux-amd64/rclone /usr/local/bin/rclone`
- Run `rclone config` and add your [cloud storage providers](https://rclone.org/overview/). Recommend Backblaze B2 for backups/snapshots and Dropbox for log files as they require link sharing support.
- Copy `config.sample` to `config` and configure
- Install WP-CLI: [Refer to Offical Docs](https://make.wordpress.org/cli/handbook/installing/)
- Install JSON package: `sudo npm install --global json`
- Install NGINX: `sudo apt-get install nginx && sudo systemctl enable nginx.service && sudo systemctl start nginx.service`
- Install MariaDB: `sudo apt-get install mariadb-server mariadb-client && sudo systemctl enable mysql.service && sudo systemctl start mysql.service && sudo mysql_secure_installation`
- Copy MariaDB root password to `~/.captaincore-cli/config` as `local_wp_db_pw="<db-password>"`

## Usage

### How site names work

CaptainCore uses arbitrary site names. When managing multiple sites, there needs to be a way to uniquely identify each site. While domain names seems like a good option, there are times you may want to be managing the same site with the same domain on multiple host providers. Also domain names can be long and sometimes change. On of flip side using a completely arbitrary site ID isn't very human friendly. A site name is something in between. A short but meaningful name that is unchangeable even if the domain name changes.

Site names can also specify a provider using an @ symbol `<site>@<provider>`. This makes dealing with multiple host providers enjoyable. Here's an example coping a site between providers `captaincore copy anchorhost@wpengine anchorhost@kinsta`. Omitting the provider is completely valid however won't be very particular if multiple site names exist.

## Commands

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

## Real World Examples

Downgrade WooCommerce on sites only running speific WooCommerce version

`captaincore ssh $(captaincore site list --filter=plugin --filter-name=woocommerce --filter-version=3.3.0) --command="wp plugin install woocommerce --version=3.2.6"`

Backup all sites

`captaincore backup --all`

Generate quicksave for all sites

`captaincore quicksave --all`

Launch site. Will change default Kinsta/WP Engine urls to real domain name and drop search engine privacy.

`captaincore ssh <site-name> --script=launch --domain=<domain>`

Find and replace http to https urls

`captaincore ssh <site-name> --script=applyhttps`

## License
This is free software under the terms of MIT the license (check the LICENSE file included in this package).
