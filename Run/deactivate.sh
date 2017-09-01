#!/bin/bash

##
##      Deploys custom deactivate mu-plugin to batch of installs
##
##      Pass arguments from command line like this
##      Script/Run/deactivate.sh install1 install2
##


### Load configuration
source ~/Scripts/config.sh

# Paths
local_path="$path/anchor.host/deploy"
plugin="anchorhost_deactivated"

deactivate () {
for (( i = 1; i <= $#; i++ ))
do
    var="$i"
    website=${!var}

    ### Load FTP credentials
		source $path_scripts/logins.sh

    ### Credentials found, start the backup
    if ! [ -z "$domain" ]
    then

      ### upload deactivation plugin to mu-plugins
      lftp -e "set sftp:auto-confirm yes;set net:max-retries 2;set net:reconnect-interval-base 5;set net:reconnect-interval-multiplier 1;put -O $homedir/wp-content/mu-plugins/ $local_path/$plugin.php; exit" -u $username,$password -p $port $protocol://$ipAddress

      echo "deactivated $domain"

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
    deactivate $*
fi
