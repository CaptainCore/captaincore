### Load configuration
source ~/Scripts/config.sh

if [ $# -gt 0 ]
then
	## Prep new config files
	mv ~/Tmp/logins.sh ~/Scripts/
	chmod +x ~/Scripts/logins.sh

	## Generates final snapshot archive
	timedate=$(date +%Y-%m-%d)
	tar -cvzf $path_tmp/$1-$timedate.tar.gz -C ~/Backup/ $1/

	## Moves snapshot to Dropbox archive folder
	$path_rclone/rclone move $path_tmp/$1-$timedate.tar.gz Anchor-Dropbox:Backup/Archive/

	## Removes directory from backup server
	rm -rf ~/Backup/$1
fi
