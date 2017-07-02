#!/bin/bash

generate_admin () {
for (( i = 1; i <= $#; i++ ))
do
    var="$i"
    website=${!var}
    
    if [[ $website == *"="* ]]
    then
      ## assume its a command
      echo "running command: $website";
      group=${website##*=}
    else
      echo $website
      echo $group
    fi

done

}

### See if any specific sites are selected
if [ $# -gt 0 ]
then
    ## Run selected installs
    generate_admin $*
fi