
# Load today's file log
dropbox_log=`ls Logs/$(date +'%Y-%m-%d')/01-05-*/backup-dropbox.txt`
backup_log=`ls Logs/$(date +'%Y-%m-%d')/01-05-*/backup-log.txt`

# Output the file name and a line break
printf "Selected Dropbox Log: \e[1;32m$dropbox_log\e[0m\n"

# Calculate Dropbox transfer
php ~/Scripts/calculate_transferred.php file=$dropbox_log

# Output bottom of backup log
printf "\nSelected Backup Log: \e[1;32m$backup_log\e[0m\n"
tail -4 $backup_log
