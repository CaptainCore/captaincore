#!/bin/bash

##
##      Checks progress of backup
##
##      Check the most recent backup
##      Script/Run/progress.sh
##
##      Or a specific backup
##      Script/Run/backup.sh 2017-07-04 02-30-d39f6c2
##

if [ $# -gt 0 ]
then
	# Load selected backup logs
	most_recent_date=$1
  most_recent_log=$2
else
  # Load most recent file log
  most_recent_date=`ls -d ~/Logs/*/ | xargs -n 1 basename | tail -1`

  # most_recent_date=`ls -rt ~/Logs/ | tail -1`
  most_recent_log=`ls -rt ~/Logs/$most_recent_date/ | tail -1`
fi


backup_log=$( { ls ~/Logs/$most_recent_date/$most_recent_log/backup-log.txt; } 2>&1 )
local_log=$( { ls ~/Logs/$most_recent_date/$most_recent_log/backup-local.txt; } 2>&1 )
remote_log=$( { ls ~/Logs/$most_recent_date/$most_recent_log/backup-remote.txt; } 2>&1 )
b2_log=$( { ls ~/Logs/$most_recent_date/$most_recent_log/backup-b2.txt; } 2>&1 )

if [[ "$backup_log" != *"No such file or directory"* ]]; then

  # Output bottom of backup log
  printf "Selected Backup Log: \e[1;32m$backup_log\e[0m\n"
  tail -4 $backup_log
  printf "\n"

fi

# current FTP backup
site_backup=`ls -rt ~/Logs/$most_recent_date/$most_recent_log/site-* | tail -1 | xargs -n 1 basename`
site_backup_log=$( { ls ~/Logs/$most_recent_date/$most_recent_log/$site_backup; } 2>&1 )

# Output sync status
printf "Selected Site: \e[1;32m$site_backup_log\e[0m\n"
calc_site_backup_log=`php ~/Scripts/Get/log_stat.php log=$site_backup_log`
echo $calc_site_backup_log
printf "\n"

if [[ "$b2_log" != *"No such file or directory"* ]]; then

  # Output the file name and a line break
  printf "Selected b2 Log: \e[1;32m$b2_log\e[0m\n"

  # Calculate B2 transfer
  calc_b2=`php ~/Scripts/Get/restic_stats.php log=$b2_log`
  echo $calc_b2
	printf "\n"

fi

if [[ "$local_log" != *"No such file or directory"* ]]; then

  # Output the log folder name
  printf "Selected Local Log: \e[1;32m$local_log\e[0m\n"

  # Calculate log stats
	calc_log_stats=`php ~/Scripts/Get/transferred_stats.php file=$local_log`
  #calc_log_stats=`php ~/Scripts/Get/log_stats.php log=$local_log`
  echo $calc_log_stats
	printf "\n"

fi

if [[ "$remote_log" != *"No such file or directory"* ]]; then

  # Output the file name and a line break
  printf "Selected Remote Log: \e[1;32m$remote_log\e[0m\n"

  # Calculate Remote transfer
  calc_remote=`php ~/Scripts/Get/transferred_stats.php file=$remote_log`
  echo $calc_remote
	printf "\n"

fi
