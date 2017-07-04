#!/bin/bash

# Load configuration
source ~/Scripts/config.sh

# Loop through arguments and seperate regular arguments from flags (--flag)
for var in "$@"
do
	# If starts with "--" then assign it to a flag array
    if [[ $var == --* ]]
    then
    	count=1+${#flags[*]}
    	flags[$count]=$var
    # Else assign to an arguments array
    else
    	count=1+${#arguments[*]}
    	arguments[$count]=$var
    fi
done

# Loop through flags and assign to varible. A flag "--skip-dropbox" becomes $flag_skip_dropbox
for i in "${!flags[@]}"
do

	# replace "-" with "_" and remove leading "--"
	flag_name=`echo ${flags[$i]} | tr - _`
	flag_name=`echo $flag_name | cut -c 3-`

	# assigns to $flag_flagname
	declare "flag_$flag_name"=true

done

# See if any specific sites are selected
backup_install () {
if [ $# -gt 0 ]; then

	INDEX=1
	for website in "$@"
	do

		### Load FTP credentials
		source $path_scripts/logins.sh

		### Credentials found, start the backup
		if ! [ -z "$domain" ]
		then

			if [ "$homedir" == "" ]
			then
			   	homedir="/"
			fi

			# captures FTP errors in $ftp_output and file listing within file called ftp_ls
			ftp_output=$( { lftp -e "set sftp:auto-confirm yes;set net:max-retries 2;set ftp:ssl-allow no; ls; exit" -u $username,$password -p $port $protocol://$ipAddress > $path_tmp/ftp_ls; } 2>&1 )

			# Handle FTP errors
			if [ -n "$ftp_output" ]
			then
				## Add FTP error to log file
				echo "FTP response: $website ($ftp_output)"
			else

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
		username=''
		password=''
		ipAddress=''
		protocol=''
		port=''
		homedir=''
		remoteserver=''

		let INDEX=${INDEX}+1
	done

fi
}

### See if any specific sites are selected
if [ ${#arguments[*]} -gt 0 ]
then
	# Backup selected installs
	backup_install ${arguments[*]}
else
	# Backup all installs
	backup_install ${websites[@]}
fi
