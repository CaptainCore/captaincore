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

// Fetch site details
$site_details = json_decode( shell_exec( "captaincore site get $site --captain_id=$captain_id" ) );

// Determines environment
if ( strpos($site, '-staging') !== false ) {
    $environment = "staging";
    $site_name = shell_exec( "captaincore ssh $site --command=\"wp option get home --skip-plugins --skip-themes\"" );
    $site_name = str_replace( "http://", "", $site_name );
    $site_name = trim ( str_replace( "https://", "", $site_name ) );
} else {
    $environment = "production";
    $site_name = $site_details->domain;
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

if ( $system->captaincore_dev == true ) {
    $fathom_instance = "http://{$configuration->vars->captaincore_tracker}";
} else {
    $fathom_instance = "https://{$configuration->vars->captaincore_tracker}";
}

$login_details = array( 
    'email'    => $configuration->vars->captaincore_tracker_user, 
    'password' => $configuration->vars->captaincore_tracker_pass
);

// Authenticate to Fathom
$auth = wp_remote_post( "$fathom_instance/api/session", array( 
    'method'  => 'POST',
    'headers' => array( 'Content-Type' => 'application/json; charset=utf-8' ),
    'body'    => json_encode( $login_details )
) );

// Add a new site to Fathom
$response = wp_remote_post( "$fathom_instance/api/sites", array( 
    'cookies' => $auth['cookies'] ,
    'headers' => array( 'Content-Type' => 'application/json; charset=utf-8' ),
    'body'    => json_encode( array ( 'name' => $site_name ) )
) );

$new_code = json_decode( $response['body'] )->Data;
$tracking_code = "[{\"code\":\"{$new_code->trackingId}\",\"domain\":\"{$site_name}\"}]";

// Store updated info in WordPress datastore
if ( $environment == "production" ) {
    echo shell_exec( "wp post meta update {$site_details->ID} fathom '$tracking_code'");
}
if ( $environment == "staging" ) {
    echo shell_exec( "wp post meta update {$site_details->ID} fathom_staging '$tracking_code'");
}

// Deploy tracker
echo shell_exec( "captaincore stats-deploy $site '$tracking_code' --captain_id=$captain_id" );
