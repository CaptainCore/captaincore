=== CaptainCore CLI ===
Contributors: austinginder
License: The MIT License

== Description ==
Collection of scripts for automating repeat WordPress tasks. Built using [Bash CLI](https://github.com/SierraSoftworks/bash-cli).

= Website =
https://anchor.host

# Getting started

* Copy config.sample.sh to config and configure with appropriate folders
* Run `Scripts/dropbox_uploader.sh` and config with Dropbox account
* Run `rclone config` and config Dropbox account

# Usage

	Format
	{Action}/{Task}.sh {installname}

	Possible Actions:
	Get, Run, Delete


		# Adds website
		php Run/new.php install=elevatedplumbi domain=elevatedplumbingandair.com username=elevatedplumbi password=***REMOVED*** address=elevatedplumbi.wpengine.com protocol=sftp port=2222 preloadusers=2823 homedir= token=***REMOVED***

		# Removes website
		php Delete/install.php install=anchorhosting domain=anchor.host

		# Backup website
		Run/Backup.sh installname

		# Generate backup snapshot
		Run/snapshot.sh installname

		# Get stats
		Get/stats.sh installname
