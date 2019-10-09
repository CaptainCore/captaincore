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

$manifest_path = $system->path;
if ( $system->captaincore_fleet == "true" ) {
	$manifest_path = "{$manifest_path}/{$captain_id}";
}

foreach($config_data as $config) {
	if ( isset( $config->captain_id ) and $config->captain_id == $captain_id ) {
		$configuration = $config;
		break;
	}
}

if ( file_exists($manifest_path . "/manifest.json") ) {
	$configuration->vars->manifest = json_decode ( file_get_contents ( $manifest_path . "/manifest.json" ) );
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

if ( $command == "fetch-captain_ids" ) {

	$captain_ids = array(); 

	foreach($config_data as $config) {
		if ( isset( $config->captain_id ) ) {
			$captain_ids[] = $config->captain_id;
		}
	}

	echo implode( " ", $captain_ids );

}