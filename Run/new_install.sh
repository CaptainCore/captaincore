#!/bin/bash

## Prep new config files
mv ~/Tmp/logins.sh ~/Scripts/
chmod +x ~/Scripts/logins.sh

## load custom configs into wp-config.php and .htaccess, setups up token
~/Scripts/Run/backup_init.sh $1
echo "load custom configs into wp-config.php and .htaccess"
echo "Setting up token"

## loads users
~/Scripts/Run/load_users.sh $1
echo "Preload Users"

## install plugins
~/Scripts/Run/load_plugins.sh $1
echo "Preload Plugins"

## run initial backup
~/Scripts/Run/backup.sh $1
echo "Running Initial Backup"
