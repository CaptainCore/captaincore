#!/bin/bash

##
##      Removes custom deactivate mu-plugin to batch of installs
##
##      Pass arguments from command line like this
##      Script/Run/activate.sh install1 install2
##


### Load configuration
source ~/Scripts/config.sh

# Paths
plugin="anchorhost_deactivated"

activate () {
for (( i = 1; i <= $#; i++ ))
do
    var="$i"
    website=${!var}

    ### Load FTP credentials
		source $path_scripts/logins.sh

    ### Credentials found, start the backup
    if ! [ -z "$domain" ]
    then

      ### remove deactivation plugin
      lftp -e "set sftp:auto-confirm yes;set net:max-retries 2;set ftp:ssl-allow no;rm $homedir/wp-content/mu-plugins/$plugin.php; exit" -u $username,$password -p $port $protocol://$ipAddress

      echo "activated $domain"
      
    fi

    ### Clear out variables
    domain=''
    username=''
    password=''
    ipAddress=''
    protocol=''
    port=''

done

}

### See if any specific sites are selected
if [ $# -gt 0 ]
then
    ## Run selected installs
    activate $*
fi
