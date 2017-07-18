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
##      --use-local-lftp (Pull) Use lftp incremental sync instead of rclone
##      --use-restic     (Push) Save to restic B2 backup
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

			# captures FTP errors in $ftp_output and file listing to log file
			ftp_output=$( { lftp -e "set sftp:auto-confirm yes;set net:max-retries 2;set ftp:ssl-allow no; ls; exit" -u $username,$password -p $port $protocol://$ipAddress > $logs_path/backup-ftp-ls.txt; } 2>&1 )

			# Handle FTP errors
			if [ -n "$ftp_output" ]
			then
				## Add FTP error to log file
				echo "FTP response: $website ($ftp_output)<br>" >> $logs_path/backup-log.txt
			else
				## No errors found, run the backup

        timebegin=$(date +"%s")

        ### Incremental backup locally with lftp
        if [[ $flag_use_local_lftp == true ]]; then

          echo "$(date +'%Y-%m-%d %H:%M') Begin incremental backup $website to local (${INDEX}/$#)" >> $logs_path/backup-log.txt

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

        else
        ### Incremental backup locally with rclone

          ### Lookup rclone
          remotes=$($path_rclone/rclone listremotes)

          ### Check for rclone remote
          rclone_remote_lookup=false
          for item in ${remotes[@]}; do
              if [[ sftp-$website: == "$item" ]]; then
                rclone_remote_lookup=true
              fi
          done

          if [[ $rclone_remote_lookup == false ]]; then
            echo "Generating rclone configs for $website"
            hashed_password=$(go run ~/Scripts/Get/pw.go $password)
            php $path_scripts/Run/rclone_import.php install=$website address=$ipAddress username=$username password=$hashed_password protocol=$protocol port=$port
          fi

  				## Database backup (if remote server available)
  				if [ -n "$remoteserver" ]
  				then
  					remoteserver="$username@$ipAddress -p $port"
  				  ssh $remoteserver '~/scripts/db_backup.sh'
  				fi

          $path_rclone/rclone sync sftp-$website:$homedir $path/$domain/ --exclude .DS_Store --verbose=1 --log-file="$logs_path/site-$website.txt"
  				timeend=$(date +"%s")
  				diff=$(($timeend-$timebegin))
  				echo "" >> $logs_path/site-$website.txt
          tail $logs_path/site-$website.txt >> $logs_path/backup-local.txt
  				echo "$(($diff / 60)) minutes and $(($diff % 60)) seconds elapsed." >> $logs_path/site-$website.txt
        fi

        if [[ "$OSTYPE" == "linux-gnu" ]]; then
            ### Begin folder size in bytes without apparent-size flag
            folder_size=`du -s --block-size=1 $path/$domain/`
            folder_size=`echo $folder_size | cut -d' ' -f 1`

        elif [[ "$OSTYPE" == "darwin"* ]]; then
            ### Calculate folder size in bytes http://superuser.com/questions/22460/how-do-i-get-the-size-of-a-linux-or-mac-os-x-directory-from-the-command-line
            folder_size=`find $path/$domain/ -type f -print0 | xargs -0 stat -f%z | awk '{b+=$1} END {print b}'`
        fi

        ### Incremental backup upload to Dropbox

        timebegin=$(date +"%s")
        echo "$(date +'%Y-%m-%d %H:%M') Begin incremental backup $website to Dropbox (${INDEX}/$#)" >> $logs_path/backup-log.txt
        $path_rclone/rclone sync $path/$domain Anchor-Dropbox:Backup/Sites/$domain -v --exclude .DS_Store --transfers=1 --stats=5m --log-file="$logs_path/site-$website-dropbox.txt"

        ### Add install to Dropbox log file
        echo "$(date +'%Y-%m-%d %H:%M') Finished incremental backup $website to Dropbox (${INDEX}/$#)" >> $logs_path/backup-dropbox.txt

        ### Grabs last 6 lines of output from dropbox transfer to log file
        tail -6 $logs_path/site-$website-dropbox.txt >> $logs_path/backup-dropbox.txt

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

  ### Start Restic backup to B2
  if [[ $flag_use_restic == true ]]; then

    ### Begin restic snapshot
    restic_output=$( { $path_restic/restic -r b2:AnchorHost:Restic backup ~/Backup/ > $logs_path/backup-restic.txt; } 2>&1 )

    ### Check for new snapshot
    if [[ "$OSTYPE" == "linux-gnu" ]]; then
        restic_snapshot=`tail $logs_path/backup-restic.txt | grep -oP '[^\s]+(?= saved)'`

    elif [[ "$OSTYPE" == "darwin"* ]]; then
        restic_snapshot=`tail $logs_path/backup-restic.txt | ggrep -oP '[^\s]+(?= saved)'`
    fi

    if [[ "$restic_snapshot" != "" ]]; then
      ### Snapshot found, add snapshot to Anchor backend
      curl "https://anchor.host/anchor-api/anchor.host/?storage=$folder_size&archive=$restic_snapshot&token=$token"
      echo "$restic_output"
      echo "$restic_output" >> $logs_path/backup-b2.txt
      echo "$(date +'%Y-%m-%d %H:%M') Finished restic backup $website to B2 (${INDEX}/$#)" >> $logs_path/backup-b2.txt
    else
      ### Snapshot not found, add error to backup-b2.txt log
      echo "$(date +'%Y-%m-%d %H:%M') Failed restic backup $website to B2 (${INDEX}/$#): $restic_output" >> $logs_path/backup-b2.txt
    fi

  fi



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
