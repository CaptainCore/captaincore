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

$site           = ( new CaptainCore\Sites )->get( $site_id );
$environment_id = ( new CaptainCore\Site( $site_id ) )->fetch_environment_id( $environment );

if ( $user_id == "") {
    $user_id = "0";
}

$time_now = date("Y-m-d H:i:s");
$in_24hrs = date("Y-m-d H:i:s", strtotime ( date("Y-m-d H:i:s")."+24 hours" ) );
$token    = bin2hex( openssl_random_pseudo_bytes( 16 ) );
$snapshot = [
    'user_id'        => $user_id,
    'site_id'        => $site_id,
    'environment_id' => $environment_id,
    'snapshot_name'  => $archive,
    'created_at'     => $time_now,
    'storage'        => $storage,
    'email'          => $email,
    'notes'          => $notes,
    'expires_at'     => $in_24hrs,
    'token'          => $token
];

$snapshot['snapshot_id'] = ( new CaptainCore\Snapshots )->insert( $snapshot );

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

// Prepare request to API
$request = [
    'method'  => 'POST',
    'headers' => [ 'Content-Type' => 'application/json' ],
    'body'    => json_encode( [ 
        "command" => "snapshot-add",
        "site_id" => $site->site_id,
        "token"   => $configuration->keys->token,
        "data"    => $snapshot,
    ] ),
];

if ( ! empty( $system->captaincore_dev ) ) {
    $request['sslverify'] = false;
}

// Post to CaptainCore API
$response = wp_remote_post( $configuration->vars->captaincore_api, $request );
echo $response['body'];

$environment             = ( new CaptainCore\Environments )->get( $environment_id );
$details                 = json_decode( $environment->details );
$details->snapshot_count = count ( ( new CaptainCore\Snapshots )->where( [ "environment_id" => $environment_id ] ) );

$environment_update = [
    "environment_id" => $environment_id,
    "details"        => json_encode( $details ),
    "updated_at"     => date("Y-m-d H:i:s"),
];

( new CaptainCore\Environments )->update( $environment_update, [ "environment_id" => $environment_id ] );

// Prepare request to API
$request = [
    'method'  => 'POST',
    'headers' => [ 'Content-Type' => 'application/json' ],
    'body'    => json_encode( [ 
        "command" => "update-environment",
        "site_id" => $site_id,
        "token"   => $configuration->keys->token,
        "data"    => $environment_update,
    ] ),
];

if ( $system->captaincore_dev ) {
    $request['sslverify'] = false;
}

// Post to CaptainCore API
$response = wp_remote_post( $configuration->vars->captaincore_api, $request );
