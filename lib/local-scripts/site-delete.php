<?php

$captain_id = getenv('CAPTAIN_ID');

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
$json = "{$_SERVER['HOME']}/.captaincore/config.json";

if ( ! file_exists( $json ) ) {
	echo "Error: Configuration file not found.";
	return;
}

$config_data = json_decode ( file_get_contents( $json ) );
$system      = $config_data[0]->system;
$path        = $system->path;

foreach($config_data as $config) {
	if ( isset( $config->captain_id ) and $config->captain_id == $captain_id ) {
		$configuration = $config;
		break;
	}
}

$site = ( new CaptainCore\Sites )->get( $site_id );

if ( $site ) {
	( new CaptainCore\Sites )->delete( $site_id );

	// Prepare request to API
	$request = [
		'method'  => 'POST',
		'headers' => [ 'Content-Type' => 'application/json' ],
		'body'    => json_encode( [ 
			"command" => "site-delete",
			"site_id" => $site_id,
			"token"   => $configuration->keys->token,
		] ),
	];

	if ( ! empty( $system->captaincore_dev ) ) {
		$request['sslverify'] = false;
	}

	// Post to CaptainCore API
	$response = wp_remote_post( $configuration->vars->captaincore_api, $request );
	echo json_decode( $response['body'] );
}

