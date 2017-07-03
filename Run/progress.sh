# Load most recent file log
most_recent_date=`ls -d ~/Logs/*/ | xargs -n 1 basename | tail -1`
# most_recent_date=`ls -rt ~/Logs/ | tail -1`
most_recent_log=`ls -rt ~/Logs/$most_recent_date/ | tail -1`

dropbox_log=`ls ~/Logs/$most_recent_date/$most_recent_log/backup-dropbox.txt`
backup_log=`ls ~/Logs/$most_recent_date/$most_recent_log/backup-log.txt`
b2_log=`ls ~/Logs/$most_recent_date/$most_recent_log/backup-b2.txt`

# current FTP backup
site_backup=`ls -rt ~/Logs/$most_recent_date/$most_recent_log/site-* | tail -1 | xargs -n 1 basename`

# Output bottom of backup log
printf "Selected Backup Log: \e[1;32m$backup_log\e[0m\n"
tail -4 $backup_log
printf "\n"

# Output sync status
printf "Selected Site: \e[1;32m$site_backup\e[0m\n"
tail -5 ~/Logs/$most_recent_date/$most_recent_log/$site_backup
printf "\n"

# Output the file name and a line break
printf "Selected b2 Log: \e[1;32m$b2_log\e[0m\n"

# Calculate B2 transfer
calc_b2=`php ~/Scripts/Get/restic_stats.php log=$b2_log`
echo $calc_b2 | awk '{gsub("<br>","\n")};1'

# Output the log folder name
printf "Selected Log Folder: \e[1;32m~/Logs/$most_recent_date/$most_recent_log/\e[0m\n"

# Calculate log stats
calc_log_stats=`php ~/Scripts/Get/log_stats.php log=~/Logs/$most_recent_date/$most_recent_log/`
echo $calc_log_stats | awk '{gsub("<br>","\n")};1'

# Output the file name and a line break
printf "Selected Dropbox Log: \e[1;32m$dropbox_log\e[0m\n"

# Calculate Dropbox transfer
calc_dropbox=`php ~/Scripts/Get/transferred_stats.php file=$dropbox_log`
echo $calc_dropbox | awk '{gsub("<br>","\n")};1'
