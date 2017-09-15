#!/bin/bash

##
##      Batch backup of WordPress sites
##
##      Pass arguments from command line like this
##      Script/Run/captaincore.sh install1 install2
##
##      Or backup everything like this
##      Script/Run/captaincore.sh
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

captaincore () {
if [ $# -gt 0 ]; then

	INDEX=1
	for website in "$@"
	do

		### Load FTP credentials
		source $path_scripts/logins.sh

		### Credentials found, start the backup
		if ! [ -z "$domain" ]
		then

      core=`~/Scripts/Run/sshwpe.sh $website "wp core version"`
      themes=`~/Scripts/Run/sshwpe.sh $website "wp theme list --fields=name,title,status,update,version --format=json"`
      plugins=`~/Scripts/Run/sshwpe.sh $website "wp plugin list --fields=name,title,status,update,version --format=json"`

      # Post CaptainCore info
      curl -g "https://anchor.host/anchor-api/$domain/?core=$core&themes=$themes&plugins=$plugins&token=$token"
      echo "https://anchor.host/anchor-api/$domain/?core=$core&themes=$themes&plugins=$plugins&token=$token"

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
    s3bucket=''
    s3path=''

		let INDEX=${INDEX}+1
	done

fi
}

### See if any specific sites are selected
if [ ${#arguments[*]} -gt 0 ]
then
	# Backup selected installs
	captaincore ${arguments[*]}
else
	# Backup all installs
	captaincore ${websites[@]}
fi
