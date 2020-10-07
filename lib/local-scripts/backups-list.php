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

foreach($config_data as $config) {
	if ( isset ( $config->captain_id ) and $config->captain_id == $captain_id ) {
		$configuration = $config;
		break;
	}
}

if ( $system->captaincore_fleet == "true" ) {
    $system->rclone_backup = "{$system->rclone_backup}/{$captain_id}";
}

$command   = "restic snapshots --repo rclone:{$system->rclone_backup}/${site}_${site_id}/${environment}/restic-repo --password-file {$_SERVER['HOME']}/.captaincore-cli/data/restic.key --json";
$response  = shell_exec( $command );
$snapshots = json_decode ( $response );

foreach ( $snapshots as $snapshot ) {
    unset( $snapshot->hostname );
    unset( $snapshot->username );
    unset( $snapshot->paths );
    unset( $snapshot->uid );
    unset( $snapshot->gid );
}
echo json_encode( $snapshots, JSON_PRETTY_PRINT );

if ( empty ( count( $snapshots ) ) ) {
    $error = [
        "response" => $response,
        "command"  => $command,
    ];
    $error_file = "{$_SERVER['HOME']}/.captaincore-cli/data/snapshot-error.log";
    file_put_contents( $error_file, json_encode( $error, JSON_PRETTY_PRINT ) );
}

$environment_id = ( new CaptainCore\Site( $site_id ) )->fetch_environment_id( $environment );
$environment    = ( new CaptainCore\Environments )->get( $environment_id );

$details               = json_decode( $environment->details );
$details->backup_count = count( $snapshots );

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