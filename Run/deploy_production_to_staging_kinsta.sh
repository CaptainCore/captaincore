#!/bin/bash

##
##      Deploy Kinsta's production to staging
##
##      Pass arguments from command line like this
##      Scripts/Run/deploy_production_to_staging_kinsta.sh anchorhost
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

deploy_staging () {
if [ $# -gt 0 ]; then

	echo "Deploying $# staging sites"
	INDEX=1
	for website in "$@"
	do

		### Load FTP credentials
		source $path_scripts/logins.sh

		### Credentials found, start the backup
		if ! [ -z "$domain" ]; then

			if [ "$homedir" == "" ]; then
			   	homedir="/"
			fi

      # Prep ssh info for production and staging sites
      remoteserver_production="-oStrictHostKeyChecking=no $username@$ipAddress -p $port"
      remoteserver_staging="-oStrictHostKeyChecking=no $staging_username@$staging_ipAddress -p $staging_port"

      # Sync production to staging
      $path_rclone/rclone sync sftp-$website:$homedir/wp-content/ sftp-$website-staging:$homedir/wp-content/ --exclude .DS_Store --exclude *timthumb.txt --exclude /wp-content/uploads_from_s3/ --verbose=1

      # Make database backup on production
      ssh $remoteserver_production 'cd public/ && wp db export --skip-plugins --skip-themes --add-drop-table - > wp-content/mysql.sql'

      # Sync production to staging
      $path_rclone/rclone sync sftp-$website:$homedir/wp-content/ sftp-$website-staging:$homedir/wp-content/ --exclude .DS_Store --exclude *timthumb.txt --exclude /wp-content/uploads_from_s3/ --verbose=1

      # Import database on staging
      ssh $remoteserver_staging 'cd public/ && wp db import wp-content/mysql.sql --skip-plugins --skip-themes'

      # Find and replace urls
      ssh $remoteserver_staging "cd public/ && wp search-replace //$domain //staging-$website.kinsta.com --all-tables --skip-plugins --skip-themes"
      ssh $remoteserver_staging "cd public/ && wp search-replace //www.$domain //staging-$website.kinsta.com --all-tables --skip-plugins --skip-themes"

      # Enable search privacy
      ssh $remoteserver_staging "cd public/ && wp option update blog_public 0 --skip-plugins --skip-themes"

      # Disable email on staging site
      ssh $remoteserver_staging 'cd public/ && wp plugin install log-emails disable-emails --skip-plugins --skip-themes && wp plugin activate log-emails disable-emails --skip-plugins --skip-themes'

		fi

		### Clear out variables
		domain=''
		username=''
		password=''
		ipAddress=''
		protocol=''
		port=''
    staging_username=''
    staging_password=''
    staging_ipAddress=''
    staging_protocol=''
    staging_port=''
    preloadusers=''
		homedir=''
		remoteserver=''
    s3bucket=''
    s3path=''
    subsite=''

		let INDEX=${INDEX}+1
	done

fi
}

### See if any specific sites are selected
if [ ${#arguments[*]} -gt 0 ]; then
	# Backup selected installs
	deploy_staging ${arguments[*]}
else
	# Backup all installs
	deploy_staging ${websites[@]}
fi
