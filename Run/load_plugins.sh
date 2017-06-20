### Load configuration 
source ~/Scripts/config.sh

# Paths
local_path="~/Backup/anchor.host/wp-content/plugins"
remote_path="/wp-content/plugins"

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
			for plugin in akismet worker jetpack anchorhost-client gravityforms google-analytics-for-***REMOVED***
			do
				### Incremental backup download to local file system
				lftp -e "set sftp:auto-confirm yes;set net:max-retries 2;set net:reconnect-interval-base 5;set net:reconnect-interval-multiplier 1;mirror --only-newer --delete --reverse --parallel=2 --exclude .git/ --exclude .DS_Store --exclude Thumbs.db --verbose=2 $local_path/$plugin $remote_path/$plugin; exit" -u $username,$password -p $port $protocol://$ipAddress
			done

			# mail -s "Plugins loaded: $domain | $(date +'%Y-%m-%d')" support@anchor.host
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
