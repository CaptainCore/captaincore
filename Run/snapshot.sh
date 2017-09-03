### Load configuration
#
#	Usage: Script/snapshot.sh anchor.host
#
source ~/Scripts/config.sh

if [ $# -gt 0 ]
then

	## Generates snapshot archive
	timedate=$(date +%Y-%m-%d)
	tar -cvz --exclude=".git" --exclude="$site/wp-content/object-cache.php" --exclude="$site/wp-content/advanced-cache.php" --exclude=".gitignore" --exclude=".gitattributes" --exclude="_wpeprivate" -f $path_tmp/$1-$timedate.tar.gz -C ~/Backup/ $1/

	### Grab snapshot size in bytes
	if [[ "$OSTYPE" == "linux-gnu" ]]; then
	    ### Begin folder size in bytes without apparent-size flag
        snapshot_size=`du -s --block-size=1 $path_tmp/$1-$timedate.tar.gz`
        snapshot_size=`echo $snapshot_size | cut -d' ' -f 1`

	elif [[ "$OSTYPE" == "darwin"* ]]; then
        ### Calculate folder size in bytes http://superuser.com/questions/22460/how-do-i-get-the-size-of-a-linux-or-mac-os-x-directory-from-the-command-line
        snapshot_size=`find $path_tmp/$1-$timedate.tar.gz -type f -print0 | xargs -0 stat -f%z | awk '{b+=$1} END {print b}'`
	fi

	## Moves snapshot to Backblaze archive folder
	$path_rclone/rclone move $path_tmp/$1-$timedate.tar.gz Anchor-B2:AnchorHostBackup/Snapshots/$1/

	# Post snapshot to ACF field
	curl "https://anchor.host/anchor-api/$1/?storage=$snapshot_size&archive=$1-$timedate.tar.gz&token=$token"

fi
