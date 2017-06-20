#!/bin/sh

### Load configuration 
source ~/Scripts/config.sh

backup_install () {
if [ $# -gt 0 ]
then

	echo "Backing up $# installs"
	for (( i = 1; i <= $#; i++ ))
	do

		var="$i"
		website=${!var}

		### Load FTP credentials 
		source $path_scripts/logins.sh

		### Credentials found, start the backup
		if ! [ -z "$domain" ]
		then

			### Generate snapshot via Task Spooler
			ts $path_scripts/snapshot.sh $domain

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

	done

fi
}

### See if any specific sites are selected
if [ $# -gt 0 ]
then
	## Run selected installs
	backup_install $*
else
	# Run all installs
	backup_install ${websites[@]}
fi
