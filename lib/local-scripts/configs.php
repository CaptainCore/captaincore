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

$command    = $argv[1];
$captain_id = getenv('CAPTAIN_ID');

// Assign arguments to variables
foreach ($argv as $key => $argument) {
	if( strpos($argument, "--format=" ) !== false ) {
		$format = str_replace( "--format=", "", $argument );
		unset( $argv[$key] );
	}
}

$json        = $_SERVER['HOME'] . "/.captaincore/config.json";
$config_data = json_decode ( file_get_contents( $json ) );
$system      = $config_data[0]->system;

foreach($config_data as $config) {
	if ( isset( $config->captain_id ) and $config->captain_id == $captain_id ) {
		$configuration = $config;
		break;
	}
}

if ( file_exists( "{$system->path}/{$captain_id}/manifest.json" ) ) {
	$configuration->vars->manifest = json_decode ( file_get_contents ( "{$system->path}/{$captain_id}/manifest.json" ) );
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

	# Adjust path if fleet mode enabled
	if ( $system->captaincore_fleet == "true" ) {
		$system->path            = "{$system->path}/{$captain_id}";
		$system->rclone_archive  = "{$system->rclone_archive}";
		$system->rclone_backup   = "{$system->rclone_backup}/{$captain_id}";
		$system->rclone_logs     = "{$system->rclone_logs}/{$captain_id}";
		$system->rclone_snapshot = "{$system->rclone_snapshot}/{$captain_id}";
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

	$output = "";

	foreach ($system as $key => $value) {
		if ( is_array($value) ) {
			continue;
		}
		$output .= "{$key}={$value}\n";
	}

	foreach ($configuration as $config) {
		if ( ! is_object ( $config ) ) {
			continue;
		}

		foreach ($config as $key => $value) {
			if ( is_object($key) ) {
				continue;
			}
			if ( is_object($value) ) {
				continue;
			}
			if ( is_array($key) ) {
				continue;
			}
			if ( is_array($value) ) {
				continue;
			}
			$output .= "{$key}={$value}\n";
		}
	}
	
	echo $output;

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
		"response" 	 => "Configurations have been updated.",
		"captain_id" => $captain_id,
	];

	echo json_encode( $response, JSON_PRETTY_PRINT );

}

if ( $command == "fetch-captain-ids" ) {

	$captain_ids = []; 

	foreach($config_data as $config) {
		if ( isset( $config->captain_id ) ) {
			$captain_ids[] = $config->captain_id;
		}
	}

	echo implode( " ", $captain_ids );

}