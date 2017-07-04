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

backup_install () {
if [ $# -gt 0 ]; then

	### Generate random auth
	auth=''; for count in {0..6}; do auth+=$(printf "%x" $(($RANDOM%16)) ); done;

	### Begin time tracking
	overalltimebegin=$(date +"%s")
	backup_date=$(date +'%Y-%m-%d')
	backup_time=$(date +'%H-%M')

	### Define log file format
	logs_path=$logs/$backup_date/$backup_time-$auth

	### Generate log folder
	mkdir -p $logs_path

	### Begin logging
	echo "$(date +'%Y-%m-%d %H:%M') Begin server backup" > $logs_path/backup-log.txt

	echo "Backing up $# installs"
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

			### Incremental backup download to local file system
			timebegin=$(date +"%s")
			echo "$(date +'%Y-%m-%d %H:%M') Begin incremental backup $website to local (${INDEX}/$#)" >> $logs_path/backup-log.txt

			# captures FTP errors in $ftp_output and file listing within file called ftp_ls
			ftp_output=$( { lftp -e "set sftp:auto-confirm yes;set net:max-retries 2;set ftp:ssl-allow no; ls; exit" -u $username,$password -p $port $protocol://$ipAddress > $path_tmp/ftp_ls; } 2>&1 )

			# Handle FTP errors
			if [ -n "$ftp_output" ]
			then
				## Add FTP error to log file
				echo "FTP response: $website ($ftp_output)<br>" >> $logs_path/backup-log.txt
			else
				## No errors found, run the backup

        ### Incremental backup locally
        if [[ $flag_skip_local != true ]]; then
  				## Database backup (if remote server available)
  				if [ -n "$remoteserver" ]
  				then
  					remoteserver="$username@$ipAddress -p $port"
  				  ssh $remoteserver '~/scripts/db_backup.sh'
  				fi

  				### Extra LFTP commands
  				## Debug mode
  				# extras="debug -o $logs_path/site-$website-debug.txt"
  				lftp -e "set sftp:auto-confirm yes;set net:max-retries 2;set net:reconnect-interval-base 5;set net:reconnect-interval-multiplier 1;set ftp:ssl-allow no;mirror --only-newer --delete --parallel=4 --exclude .git/ --exclude .DS_Store --exclude Thumbs.db --exclude all-in-one-event-calendar/cache/ --verbose=1 $homedir $path/$domain; exit" -u $username,$password -p $port $protocol://$ipAddress >> $logs_path/site-$website.txt
  				timeend=$(date +"%s")
  				diff=$(($timeend-$timebegin))
  				echo "" >> $logs_path/site-$website.txt
          tail $logs_path/site-$website.txt >> $logs_path/backup-local.txt
  				echo "$(($diff / 60)) minutes and $(($diff % 60)) seconds elapsed." >> $logs_path/site-$website.txt
        fi

				### Incremental backup upload to Dropbox
				if [[ $flag_skip_dropbox != true ]]; then

          ### Begin log file
          > $logs_path/backup-dropbox.txt

					timebegin=$(date +"%s")
					echo "$(date +'%Y-%m-%d %H:%M') Begin incremental backup $website to Dropbox (${INDEX}/$#)" >> $logs_path/backup-log.txt
					$path_rclone/rclone sync $path/$domain Anchor-Dropbox:Backup/Sites/$domain --exclude .DS_Store --dropbox-chunk-size=128M --transfers=2 --stats=5m --verbose=1 --log-file="$logs_path/site-$website-dropbox.txt"

					### Add install to Dropbox log file
					echo "$(date +'%Y-%m-%d %H:%M') Finished incremental backup $website to Dropbox (${INDEX}/$#)" >> $logs_path/backup-dropbox.txt

					### Grabs last 6 lines of output from dropbox transfer to log file
					tail -6 $logs_path/site-$website-dropbox.txt >> $logs_path/backup-dropbox.txt

				fi

				if [[ "$OSTYPE" == "linux-gnu" ]]; then
				    ### Begin folder size in bytes without apparent-size flag
                    folder_size=`du -s --block-size=1 $path/$domain/`
                    folder_size=`echo $folder_size | cut -d' ' -f 1`

				elif [[ "$OSTYPE" == "darwin"* ]]; then
			        ### Calculate folder size in bytes http://superuser.com/questions/22460/how-do-i-get-the-size-of-a-linux-or-mac-os-x-directory-from-the-command-line
			        folder_size=`find $path/$domain/ -type f -print0 | xargs -0 stat -f%z | awk '{b+=$1} END {print b}'`
				fi

        ### Restic Snapshot to Backblaze
        if [[ $flag_skip_restic != true ]]; then

          install_env=production

          if [[ "$website" == *staging ]]; then
            install_env=staging
          fi

          restic_output=$( { $path_restic/restic -r b2:AnchorHost:Backup backup ~/Backup/$domain/ --tag $website --tag $install_env; } 2>&1 )
          restic_snapshot=`echo $restic_output | grep -oP '[^\s]+(?= saved)'`
          curl "https://anchor.host/anchor-api/$domain/?storage=$folder_size&archive=$restic_snapshot&token=$token"
          echo "$restic_output"
          echo "$restic_output" >> $logs_path/backup-b2.txt
          echo "$(date +'%Y-%m-%d %H:%M') Finished restic backup $website to B2 (${INDEX}/$#)" >> $logs_path/backup-b2.txt

        fi

				### Views for yearly stats
				views=`php $path_scripts/Get/stats.php domain=$domain`

				# Post folder size bytes and yearly views to ACF field
				curl "https://anchor.host/anchor-api/$domain/?storage=$folder_size&views=$views&token=$token"

			fi

			### Generate log
			timeend=$(date +"%s")
			diff=$(($timeend-$timebegin))
			echo "$(($diff / 60)) minutes and $(($diff % 60)) seconds elapsed." >> $logs_path/site-$website.txt
			echo "" >> $logs_path/site-$website.txt

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

	### End time tracking
	overalltimeend=$(date +"%s")
	echo "" >> $logs_path/backup-log.txt
	diff=$(($overalltimeend-$overalltimebegin))
	echo "$(date +'%Y-%m-%d %H:%M') $(($diff / 3600)) hours, $((($diff / 60) % 60)) minutes and $(($diff % 60)) seconds elapsed." >> $logs_path/backup-log.txt
	echo "" >> $logs_path/backup-log.txt

	### Generate logs
	cd $logs_path
	tar -cvzf logs.tar.gz *

	### Upload logs to Dropbox
	$path_rclone/rclone sync $logs/$backup_date/$backup_time-$auth Anchor-Dropbox:Backup/Logs/$backup_date/$backup_time-$auth --exclude .DS_Store

	### Generate dropbox link to logs
	shareurl=`$path_scripts/Run/dropbox_uploader.sh share Backup/Logs/$backup_date/$backup_time-$auth`
	shareurl=`echo $shareurl | grep -o 'https.*'`

	### Generate overall emails
	( echo "$(php $path_scripts/Get/transferred_stats.php file=$logs_path/backup-dropbox.txt)" && printf "<a href='$shareurl'>View Logs</a><br><br>" && grep -r "FTP response" $logs_path/backup-log.txt; ) \
	| mutt -e 'set content_type=text/html' -s "Backup completed: $# installs | $backup_date" -a $logs_path/backup-log.txt -- support@anchor.host

	cd ~

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
