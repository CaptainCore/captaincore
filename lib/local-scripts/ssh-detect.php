<?php

$COLOR_RED    = "\033[31m";
$COLOR_NORMAL = "\033[39m";
$site         = $args[0];
$captain_id   = getenv('CAPTAIN_ID');

// Replaces dashes in keys with underscores
foreach( $args as $index => $arg ) {
	$split = strpos( $arg, "=" );
	if ( $split ) {
		$key   = str_replace('-', '_', substr( $arg , 0, $split ) );
		$value = substr( $arg , $split, strlen( $arg ) );

		// Removes unnecessary bash quotes
		$value = trim( $value,'"' ); 				// Remove last quote 
		$value = str_replace( '="', '=', $value );  // Remove quote right after equals

		$args[$index] = $key.$value;
	} else {
		$args[$index] = str_replace('-', '_', $arg);
	}
}

// Converts --arguments into $arguments
parse_str( implode( '&', $args ), $parsed_args );

$arguments = empty( $parsed_args["arguments"] ) ? [] : $parsed_args["arguments"];

if ( empty( $arguments ) ) {
    return;
}

$arguments = base64_decode( $arguments );
$arguments = explode( " ", $arguments );
$username  = $arguments[0];
$address   = $arguments[1];
$port      = $arguments[2];

// Loads CLI configs
$json = "{$_SERVER['HOME']}/.captaincore/config.json";

if ( ! file_exists( $json ) ) {
	echo "Error: Configuration file not found.";
	return;
}

$config_data = json_decode ( file_get_contents( $json ) );
$system      = $config_data[0]->system;
$path        = $system->path;

$remote_options   = "-q -oStrictHostKeyChecking=no";

$configurations = ( new CaptainCore\Configurations )->get();
$key            = $configurations->default_key;
$remote_options = "$remote_options -oPreferredAuthentications=publickey -i $system->path_keys/${captain_id}/${key}";
$remote_server = "$remote_options $username@$address -p $port";

echo "ssh $remote_server \"cd public; pwd\"";