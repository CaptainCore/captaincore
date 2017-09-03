#!/bin/bash

# Load configuration
source ~/Scripts/config.sh

if [ $# -gt 0 ]
then
	## Prep new config files
	mv ~/Tmp/logins.sh ~/Scripts/
	chmod +x ~/Scripts/logins.sh

	## Generates final snapshot
	~/Script/Run/snapshot.sh $1

	## Removes directory from backup server
	rm -rf ~/Backup/$1
fi
