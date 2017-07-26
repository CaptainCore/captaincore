#!/bin/bash

##
##      Locate and remove duplicate backups in logins.php
##
##      Pass arguments from command line like this
##      Script/Run/remove_duplicate_backups.sh
##

# Load configuration
source ~/Scripts/config.sh

duplicate_installs=(`echo ${websites[@]} | awk '{gsub(" ","\n")};1' | awk '!($0 in seen){seen[$0];next} 1'`)

duplicate_installs_count=${#duplicate_installs[@]}

# Loop through backups and check if found in websites
for (( i=0; i<${duplicate_installs_count}; i++ ));
do

  echo "Duplicate install found: ${duplicate_installs[$i]}"

done
