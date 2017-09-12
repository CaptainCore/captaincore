#!/bin/bash

##
##      Batch assign website to correct server
##
##      Pass arguments from command line like this
##      Script/Run/assign_server.sh install1 install2
##
##      Or assign all websites to correct server
##      Script/Run/assign_server.sh
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

assign_server () {
if [ $# -gt 0 ]; then

	# Loop through all websites
	INDEX=1
	for website in "$@"
	do

		### Load FTP credentials
		source $path_scripts/logins.sh

		### Credentials found, start the backup
		if ! [ -z "$domain" ]
		then

      ## Grab current IP from host name
      dig_output=`dig $ipAddress`
      website_ip=`echo $dig_output | perl -n -e '/ANSWER SECTION: .+ IN A (.+?) /&& print $1'`

      ## map to correct server
      server_id=`php ~/Scripts/Get/server.php ip=$website_ip`

      ## curl up the findings to anchor-api
      if [[ $server_id =~ ^-?[0-9]+$ ]]
      then
        curl "https://anchor.host/anchor-api/$domain/?server=$website_ip&token=$token"
      fi

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

	cd ~

fi
}

### See if any specific sites are selected
if [ ${#arguments[*]} -gt 0 ]
then
	# Backup selected installs
	assign_server ${arguments[*]}
else
	# Backup all installs
	assign_server ${websites[@]}
fi
