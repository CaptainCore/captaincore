#!/bin/bash

# Load configuration
source ~/Scripts/config

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
