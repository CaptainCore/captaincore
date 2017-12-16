#!/bin/bash

##
##      Deploys custom preloading users mu-plugin to batch of installs
##
##      Pass arguments from command line like this
##      Scripts/Run/load_users.sh install1 install2
##
##      The users argument determines which json file to load user data from
##


### Load configuration
source ~/Scripts/config.sh

generate_admin () {
for (( i = 1; i <= $#; i++ ))
do
    var="$i"
    website=${!var}

    if [[ $website == *"="* ]]
    then
      ## assume its a command and strip out the argument as the user group
      group=${website##*=}
    else
      ### Load FTP credentials
      source ~/Scripts/logins.sh

      ### Credentials found, start the backup
      if ! [ -z "$domain" ]
      then

        ### Loads token
        website_token=$(php ~/Scripts/Get/token.php domain=$domain)

        ### Generate JSON file based on preloaded list
        php ~/Scripts/Run/users-generate-json.php token=$website_token customers=$preloadusers website=$website

        ### Generate mu-plugin file based on a predefined user group
        php ~/Scripts/Run/users.php install=$website token=$website_token customers=$preloadusers

        ### upload password plugin to mu-plugins
        lftp -e "set sftp:auto-confirm yes;set net:max-retries 2;set ftp:ssl-allow no;put -O /wp-content/mu-plugins/ ~/Tmp/anchor_load_$website.php; exit" -u $username,$password -p $port $protocol://$ipAddress

        ### Trigger website to load password
        wget --no-cache --spider $ipAddress/wp-admin/
        sleep 1
        curl -Il $ipAddress/wp-login.php

        ## remove password plugin
        lftp -e "set sftp:auto-confirm yes;set net:max-retries 2;set ftp:ssl-allow no;rm /wp-content/mu-plugins/anchor_load_$website.php; exit" -u $username,$password -p $port $protocol://$ipAddress

      fi

      ### Clear out variables
      domain=''
      username=''
      password=''
      ipAddress=''
      protocol=''
      port=''
    fi

done

}

### See if any specific sites are selected
if [ $# -gt 0 ]
then
    ## Run selected installs
    generate_admin $*
fi
