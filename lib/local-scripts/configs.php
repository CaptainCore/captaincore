#! /usr/bin/env php
<?php

#
#   Configs - for bash CLI
#
#	Fetch configurations for a captain. Used within lib/arguments
#   `configs.php fetch [<keys|remotes|vars>] [<name>] [<value>]` --captain_id=<captain_id>
#
#	Update configurations for a captain. Used within `captaincore configs`
#   `configs.php update <keys|remotes|vars> <name> <value>` --captain_id=<captain_id>
#
#	Examples:
#	php configs.php update vars websites "site1 site2 site3" 
#	php configs.php update remotes rclone_backup B2:Folder/Sites --captain_id=2
#


if ( ! isset($argv[1]) ) {
	return;
}

$command = $argv[1];
$captain_id = "1";

// Assign arguments to variables
foreach ($argv as $key => $argument) {
	if( strpos($argument, "--captain_id=" ) !== false ) {
		$captain_id = str_replace( "--captain_id=", "", $argument );
		unset( $argv[$key] );
	}
	if( strpos($argument, "--format=" ) !== false ) {
		$format = str_replace( "--format=", "", $argument );
		unset( $argv[$key] );
	}
}

$json = $_SERVER['HOME'] . "/.captaincore-cli/config.json";
$config_data = json_decode ( file_get_contents( $json ) );
$system = $config_data[0]->system;

foreach($config_data as $config) {
	if ( isset( $config->captain_id ) and $config->captain_id == $captain_id ) {
		$configuration = $config;
		break;
	}
}

if ( $command == "fetch" ) {

	if ( $captain_id == "" ) { 
		echo "Error: Please set <captain_id>";
		return;
	}

	if ( ! isset( $configuration ) ) {
		echo "Error: Captain not found.";
		return;
	}

	if ( isset($argv[2]) and isset($argv[3]) ) {
		$key   = $argv[2];
		$name  = $argv[3];
		if ( isset( $configuration->{$key}->{$name} ) ) {
			echo $configuration->{$key}->{$name};
		}
		return;
	}

	if ( isset($argv[2]) ) {
		$key   = $argv[2];
		if ( isset( $configuration->{$key} ) ) {
			echo json_encode( $configuration->{$key}, JSON_PRETTY_PRINT );
		}
		return;
	}

	// If format set to JSON
	if ( isset($format) and $format == "json" ) {
		echo json_encode( $configuration, JSON_PRETTY_PRINT );
		return;
	}

$bash = <<< heredoc
captaincore_fleet={$system->captaincore_fleet}
captaincore_dev={$system->captaincore_dev}
captaincore_master={$system->captaincore_master}
captaincore_master_port={$system->captaincore_master_port}
logs={$system->logs}
path={$system->path}
path_tmp={$system->path_tmp}
rclone_cli_backup={$system->rclone_cli_backup}
local_wp_db_pw={$system->local_wp_db_pw}
path_scripts={$system->path_scripts}
access_key={$configuration->keys->access_key}
token={$configuration->keys->token}
auth={$configuration->keys->auth}
WPCOM_API_KEY={$configuration->keys->WPCOM_API_KEY}
GF_LICENSE_KEY={$configuration->keys->GF_LICENSE_KEY}
ACF_PRO_KEY={$configuration->keys->ACF_PRO_KEY}
rclone_archive={$configuration->remotes->rclone_archive}
rclone_backup={$configuration->remotes->rclone_backup}
rclone_logs={$configuration->remotes->rclone_logs}
rclone_snapshot={$configuration->remotes->rclone_snapshot}
captaincore_branding_name={$configuration->vars->captaincore_branding_name}
captaincore_branding_title={$configuration->vars->captaincore_branding_title}
captaincore_branding_author={$configuration->vars->captaincore_branding_author}
captaincore_branding_author_uri={$configuration->vars->captaincore_branding_author_uri}
captaincore_branding_slug={$configuration->vars->captaincore_branding_slug}
captaincore_server={$configuration->vars->captaincore_server}
captaincore_tracker={$configuration->vars->captaincore_tracker}
captaincore_gui={$configuration->vars->captaincore_gui}
captaincore_api={$configuration->vars->captaincore_api}
captaincore_admin_email={$configuration->vars->captaincore_admin_email}
captaincore_admin_user={$configuration->vars->captaincore_admin_user}
websites={$configuration->vars->websites}
wpe_ssh_user={$configuration->vars->wpe_ssh_user}
heredoc;

	echo $bash;

}

if ( $command == "update" ) {

	if ( !isset($argv[2]) or !isset($argv[3]) or !isset($argv[4]) ) {
		echo "Error: Please set <captain_id> <keys|paths|remotes|vars> <name> <value>";
		return;
	}

	$key   = $argv[2];
	$name  = $argv[3];
	$value = $argv[4];

	if ( ! isset($configuration->{$key}->{$name} ) ) {
		echo "Error: There is no configuration for '{$key}->{$name}'.";
		return;
	}

	// Update value
	$configuration->{$key}->{$name} = $value;

	// Write data back
	foreach($config_data as $config) {
		if ( isset( $config->captain_id ) and $config->captain_id == $captain_id ) {
			$config = $configuration;
			break;
		}
	}
	
	// Write out file
	file_put_contents( $json , json_encode( $config_data, JSON_PRETTY_PRINT ) );

	$response = (object) [
		"reponse" 	 => "Configurations have been updated.",
		"captain_id" => $captain_id,
		$key 	   	 => array ( 
			   $name => $value
		)
	];

	echo json_encode( $response, JSON_PRETTY_PRINT );

}

if ( $command == "fetch-captain_ids" ) {

	$captain_ids = array(); 

	foreach($config_data as $config) {
		if ( isset( $config->captain_id ) ) {
			$captain_ids[] = $config->captain_id;
		}
	}

	echo implode( " ", $captain_ids );

}