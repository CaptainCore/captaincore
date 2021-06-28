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

$json = $_SERVER['HOME'] . "/.captaincore/config.json";
$config_data = json_decode ( file_get_contents( $json ) );
$system = $config_data[0]->system;

$snapshot = ( new CaptainCore\Snapshots )->get( $snapshot_id );
if ( empty( $snapshot ) ) {
    echo "Error: Snapshot not found.";
    return;
}
$name     = $snapshot->snapshot_name;
$site     = ( new CaptainCore\Sites )->get( $snapshot->site_id );

// Get new auth from B2
$account_id      = CAPTAINCORE_B2_ACCOUNT_ID;  // Obtained from your B2 account page
$application_key = CAPTAINCORE_B2_ACCOUNT_KEY; // Obtained from your B2 account page
$credentials     = base64_encode( $account_id . ':' . $application_key );
$url             = 'https://api.backblazeb2.com/b2api/v1/b2_authorize_account';

$session = curl_init( $url );

// Add headers
$headers   = [];
$headers[] = 'Accept: application/json';
$headers[] = 'Authorization: Basic ' . $credentials;
curl_setopt( $session, CURLOPT_HTTPHEADER, $headers ); // Add headerss
curl_setopt( $session, CURLOPT_HTTPGET, true );        // HTTP GET
curl_setopt( $session, CURLOPT_RETURNTRANSFER, true ); // Receive server response
$server_output = curl_exec( $session );
curl_close( $session );
$output = json_decode( $server_output );

// Defines folder paths
$b2_snapshots  = CAPTAINCORE_B2_SNAPSHOTS;
$site_folder   = "{$site->site}_{$site->site_id}"; 
if ( $system->captaincore_fleet == "true" ) {
    $b2_snapshots = "{$b2_snapshots}/{$captain_id}";
}

$b2_folder_index     = strpos ( $b2_snapshots, "/" ) + 1;
$b2_snapshots_folder = substr ( $b2_snapshots, $b2_folder_index );

// Variables for Backblaze
$api_url          = 'https://api001.backblazeb2.com';       // From b2_authorize_account call
$auth_token       = $output->authorizationToken;            // From b2_authorize_account call
$bucket_id        = CAPTAINCORE_B2_BUCKET_ID;               // The file name prefix of files the download authorization will allow
$valid_duration   = 604800;                                 // The number of seconds the authorization is valid for
$file_name_prefix = "$b2_snapshots_folder/{$site_folder}";  // The file name prefix of files the download authorization will allow

$session = curl_init( $api_url . '/b2api/v1/b2_get_download_authorization' );

// Add post fields
$data = [
    'bucketId'               => $bucket_id,
    'validDurationInSeconds' => $valid_duration,
    'fileNamePrefix'         => $file_name_prefix,
];
$post_fields = json_encode( $data );
curl_setopt( $session, CURLOPT_POSTFIELDS, $post_fields );

// Add headers
$headers   = [];
$headers[] = 'Authorization: ' . $auth_token;
curl_setopt( $session, CURLOPT_HTTPHEADER, $headers );
curl_setopt( $session, CURLOPT_POST, true );           // HTTP POST
curl_setopt( $session, CURLOPT_RETURNTRANSFER, true ); // Receive server response
$server_output = curl_exec( $session );                // Let's do this!
curl_close( $session );                                // Clean up
$server_output = json_decode( $server_output );
$auth          = $server_output->authorizationToken;
$url           = "https://f001.backblazeb2.com/file/{$b2_snapshots}/{$site_folder}/{$name}?Authorization={$auth}";

echo $url;