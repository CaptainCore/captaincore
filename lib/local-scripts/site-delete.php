<?php

// Replaces dashes in keys with underscores
foreach($args as $index => $arg) {
	$split = strpos($arg, "=");
	if ( $split ) {
		$key = str_replace('-', '_', substr( $arg , 0, $split ) );
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
parse_str( implode( '&', $args ) );

// Loads CLI configs
$json = $_SERVER['HOME'] . '/.captaincore-cli/config.json';

if ( ! file_exists( $file ) ) {
	return;
}

$config_data = json_decode ( file_get_contents( $json ) );

foreach($config_data as $config) {
	if ( isset( $config->captain_id ) and $config->captain_id == $captain_id ) {
		$configuration = $config;
		break;
	}
}

// Fetch from CLI configs
$captaincore_admin_email = $configuration->vars->captaincore_admin_email;

$site = ( new CaptainCore\Sites )->get( $site_id );

if ( $site ) {
	// Make final snapshot then remove local files
	$output = shell_exec( "captaincore snapshot $site --delete-after-snapshot --email=$captaincore_admin_email --captain_id=$captain_id" );

	// Mark for delection
	( new CaptainCore\Site( $site_id, true ) )->delete();
}

