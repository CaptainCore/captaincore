#!/bin/bash

##
##      Preloading plugins to install
##
##      Pass arguments from command line like this
##      Scripts/Run/load_plugins.sh install1 install2
##

### Load configuration
source ~/Scripts/config.sh

# Paths
local_path="~/Backup/anchor.host/wp-content/plugins"

if [ $# -gt 0 ]
then
	echo "Loading specific sites"
	for (( i = 1; i <= $#; i++ ))
	do

		var="$i"
		website=${!var}

		### Load FTP credentials
		source ~/Scripts/logins.sh

		### Credentials found, start the backup
		if ! [ -z "$domain" ]
		then

			remote_path="$homedir/wp-content/plugins"

			### Loop through static list of plugins
			for plugin in akismet worker jetpack anchorhost-client gravityforms google-analytics-for-***REMOVED***
			do
				### Upload plugin
				lftp -e "set sftp:auto-confirm yes;set net:max-retries 2;set net:reconnect-interval-base 5;set net:reconnect-interval-multiplier 1;mirror --only-newer --delete --reverse --parallel=2 --exclude .git/ --exclude .DS_Store --exclude Thumbs.db --verbose=2 $local_path/$plugin $remote_path/$plugin; exit" -u $username,$password -p $port $protocol://$ipAddress
			done

			### Loads token
			website_token=$(php ~/Scripts/Get/token.php domain=$domain)

			### Load dynamic list of plugins for install
			plugins=`php ~/Scripts/Run/plugin-generate.php token=$website_token customers=$preloadusers`

			### Loop through dynamic list of plugins
			for plugin in $plugins
			do
				### Upload plugin
				lftp -e "set sftp:auto-confirm yes;set net:max-retries 2;set net:reconnect-interval-base 5;set net:reconnect-interval-multiplier 1;mirror --only-newer --delete --reverse --parallel=2 --exclude .git/ --exclude .DS_Store --exclude Thumbs.db --verbose=2 $local_path/$plugin $remote_path/$plugin; exit" -u $username,$password -p $port $protocol://$ipAddress
			done

		fi

		### Clear out variables
		domain=''
		username=''
		password=''
		ipAddress=''
		protocol=''
		port=''

	done

fi
