#!/bin/sh

# Load configuration 
source ~/Scripts/config.sh

i=1
websites=''

# See if any specific sites are selected
if [ $# -gt 0 ]; then
	echo "Loading specific sites"
	for (( i = 1; i <= $#; i++ ))
	do
		var="$i"
		website=${!var}
		websites+=$website" "
	
		### Load FTP credentials 
		source $path_scripts/logins.sh

		### Credentials found, start the backup
		if ! [ -z "$domain" ]
		then

			### Use default homepath if none is defined
			if [ "$homedir" == "" ]
			then
			   	homedir="/"
			fi

			### Pull down wp-config.php and .htaccess
			lftp -e "set sftp:auto-confirm yes;set net:max-retries 2;set net:reconnect-interval-base 5;set net:reconnect-interval-multiplier 1;mirror --only-newer --parallel=2 --exclude '.*' --exclude '.*/' --include 'wp-config.php' --include '.htaccess' --verbose=2 $homedir $path/$domain; exit" -u $username,$password -p $port $protocol://$ipAddress

			## load custom configs into wp-config.php and .htaccess
			php ~/Scripts/Get/configs.php wpconfig=$path/$domain/wp-config.php htaccess=$path/$domain/.htaccess
			sleep 1s

			### Push up modified wp-config.php and .htaccess
			lftp -e "set sftp:auto-confirm yes;set net:max-retries 2;set net:reconnect-interval-base 5;set net:reconnect-interval-multiplier 1;mirror --only-newer --reverse --parallel=2 --exclude '.*' --exclude '.*/' --include 'wp-config.php' --include '.htaccess' --verbose=2 $path/$domain $homedir; exit" -u $username,$password -p $port $protocol://$ipAddress
			
			### Generate token
			token=$(php ~/Scripts/Get/token.php domain=$domain)

			### Generate backup link
			shareurl=`$path_scripts/Run/dropbox_uploader.sh share Backup/Sites/$domain`
			shareurl=`echo $shareurl | grep -o 'https.*'`
			
			### Assign token and backup link
			curl "https://anchor.host/backup/$domain/?link=$shareurl&token=$token&auth=$auth"
			sleep 1s

		fi

		### Clear out variables
		domain=''

	done
##else
##	echo "Loading all sites"
##	### Loop through each WP Engine install
##	for website in "${websites[@]}"
##	do
##
##		### Load FTP credentials 
##		source $path_scripts/logins.sh
##
##		### Credentials found, start the backup
##		if ! [ -z "$domain" ]
##		then
##
##			### Generate token
##			token=$(php Scripts/load_wp_config.php domain=$domain)
##
##			### Generate backup link
##			shareurl=`$path_scripts/dropbox_uploader.sh share Backup/Sites/$domain`
##			shareurl=`echo $shareurl | grep -o 'https.*'`
##			
##			### Assign token and backup link
##			curl "https://anchor.host/backup/$domain/?link=$shareurl&token=$token&auth=$auth"
##			sleep 1s
##
##		fi
##
##		### Clear out variables
##		domain=''
##		i=$(($i+1))
##
##	done
##
fi
