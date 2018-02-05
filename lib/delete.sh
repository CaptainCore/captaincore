#!/bin/bash

# Load configuration
root_path="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"; root_path=${root_path%app*}
source $root_path/config

if [ $# -gt 0 ]
then
	## Prep new config files
	mv ~/Tmp/logins ~/Scripts/
	chmod +x ~/Scripts/logins

	## Generates final snapshot
	~/Script/Run/snapshot.sh $1

	## Removes directory from backup server
	rm -rf ~/Backup/$1
fi
