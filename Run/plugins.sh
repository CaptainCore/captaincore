#!/bin/bash

##
##      Deploys custom preloading users mu-plugin to batch of installs
##
##      Pass arguments from command line like this
##      Script/Run/plugin.sh install1 install2
## 
##      The users argument determines which json file to load user data from
##


### Load configuration 
source ~/Scripts/config.sh

# Paths
local_path="~/Backup/anchor.host/wp-content/plugins"
remote_path="/wp-content/plugins"

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

        ### Load plugins to install
        plugins=`php ~/Scripts/Run/plugin-generate.php token=$website_token customers=$preloadusers`

        ## Loop through each plugin and upload
        for plugin in $plugins
        do
          ### Incremental backup download to local file system
          lftp -e "set sftp:auto-confirm yes;set net:max-retries 2;set net:reconnect-interval-base 5;set net:reconnect-interval-multiplier 1;mirror --only-newer --delete --reverse --parallel=2 --exclude .git/ --exclude .DS_Store --exclude Thumbs.db --verbose=2 $local_path/$plugin $remote_path/$plugin; exit" -u $username,$password -p $port $protocol://$ipAddress
        done

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