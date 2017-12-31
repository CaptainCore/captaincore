#!/bin/bash

##
##      Kinsta and WP Engine SSH wrapper
##
##      Connects to individual install over SSH
##      Scripts/Run/sshwpe.sh anchorhost1
##
##      Runs command over SSH
##      Scripts/Run/ssh.sh anchorhost1 "wp plugins list"
##

### Load configuration
source ~/Scripts/config.sh

ssh_wrapper () {
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

        ## Database backup (if remote server available)
        if [[ "$ipAddress" == *".kinsta.com" ]]; then

          remoteserver="$username@$ipAddress -p $port"
          if [ -n "$2" ]; then
            ssh $remoteserver "cd public/ && $2"
          else
            ssh $remoteserver
          fi
        fi

        ## Database backup (if remote server available)
        if [[ "$ipAddress" == *".wpengine.com" ]]; then
          if [ -n "$2" ]; then
            ssh austin@anchor.host+$1@ssh.gcp-us-central1-farm-01.wpengine.io "cd sites/$1/ && $2"
          else
            ssh austin@anchor.host+$1@ssh.gcp-us-central1-farm-01.wpengine.io
          fi
        fi

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
    ssh_wrapper $*
fi
