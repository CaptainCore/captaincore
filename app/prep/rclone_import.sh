#!/bin/bash

##
##      Batch backup of WordPress sites
##
##      Pass arguments from command line like this
##      Script/Run/backup.sh install1 install2
##
##      Or backup everything like this
##      Script/Run/backup.sh
##
##      The following flags are also available
##      --skip-local     (Pull) Skips local incremental backup
##      --skip-dropbox   (Push) Skips remote incremental backup
##      --skip-restic    (Push) Skips remote restic backup
##

# Load configuration
source ~/Scripts/config

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

backup_install () {
if [ $# -gt 0 ]; then

	echo "Processing $# installs"
	INDEX=1
	for website in "$@"
	do

		### Load FTP credentials
		source $path_scripts/logins

		### Credentials found, start the backup
		if ! [ -z "$domain" ]
		then

			if [ "$homedir" == "" ]
			then
			   	homedir="/"
			fi

			# captures FTP errors in $ftp_output and file listing within file called ftp_ls
			ftp_output=$( { lftp -e "set sftp:auto-confirm yes;set net:max-retries 2;set ftp:ssl-allow no; ls; exit" -u $username,$password -p $port $protocol://$ipAddress; } 2>&1 )

			# Handle FTP errors
			if [ -n "$ftp_output" ]
			then
				## Add FTP error to log file
				echo "FTP response: $website ($ftp_output)"
			else
				## No errors found, import to rclone

        password_hashed=`go run $path_scripts/app/utils/pw.go $password`
        php $path_scripts/Run/rclone_import.php install=$website domain=$domain username=$username password=$password_hashed address=$ipAddress protocol=$protocol port=$post preloadusers=$preloadusers homedir=$homedir

			fi

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
