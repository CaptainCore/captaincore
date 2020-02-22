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
$json = "{$_SERVER['HOME']}/.captaincore-cli/config.json";

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

// Prepare request to API
$request = [
    'method'  => 'POST',
    'headers' => [ 'Content-Type' => 'application/json' ],
    'body'    => json_encode( [ 
        "command" => "site-get-raw",
        "site_id" => $site_id,
        "token"   => $configuration->keys->token,
    ] ),
];

if ( ! empty( $system->captaincore_dev ) ) {
    $request['sslverify'] = false;
}

// Post to CaptainCore API
$response = wp_remote_post( $configuration->vars->captaincore_api, $request );
if ( is_wp_error( $response ) ) {
    $error_message = $response->get_error_message();
    return "Something went wrong: $error_message";
}

//echo $response['body'];
$results = json_decode( $response['body'] );

$site         = $results->site;
$site_check   = ( new CaptainCore\Sites )->get( $site->site_id );
$environments = $site->environments;
$shared_with  = $site->shared_with;
unset( $site->environments );
unset( $site->shared_with );
if ( empty( $site_check ) ) {
    // Insert new site
    ( new CaptainCore\Sites )->insert( (array) $site );
    echo "Added site #{$site->site_id}";
} else {
    // update new site
    ( new CaptainCore\Sites )->update( (array) $site, [ "site_id" => $site->site_id ] );
    echo "Updating site #{$site->site_id}";
}
foreach ( $environments as $environment ) {
    $environment_id    = $environment->environment_id;
    $environment_check = ( new CaptainCore\Environments )->get( $environment_id );
    // Insert new environment
    if ( empty( $environment_check ) ) {
        ( new CaptainCore\Environments )->insert( (array) $environment );
        continue;
    }
    // Update existing environment
    ( new CaptainCore\Environments )->update( (array) $environment, [ "environment_id" => $environment_id ] );
}
foreach ( $shared_with as $record ) {
    $account_site_id    = $record->account_site_id;
    $account_site_check = ( new CaptainCore\AccountSite )->get( $account_site_id );
    // Insert new environment
    if ( empty( $account_site_check ) ) {
        ( new CaptainCore\AccountSite )->insert( (array) $record );
        continue;
    }
    // Update existing environment
    ( new CaptainCore\AccountSite )->update( (array) $record, [ "account_site_id" => $account_site_id ] );
}