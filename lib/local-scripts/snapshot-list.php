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

$json        = $_SERVER['HOME'] . "/.captaincore/config.json";
$config_data = json_decode ( file_get_contents( $json ) );
$system      = $config_data[0]->system;

$environment_id = ( new CaptainCore\Site( $site_id ) )->fetch_environment_id( $environment );
$snapshots      = ( new CaptainCore\Snapshots )->where( [ "environment_id" => $environment_id ] );
if ( empty( $snapshots ) ) {
    echo "Error: No snapshots found.";
    return;
}
if ( empty( $limit ) ) {
    $limit = "10";
}
if ( empty( $field ) ) {
    $field = "snapshot_id";
}

$snapshots = array_slice( $snapshots, 0, $limit );
$response  = array_column( $snapshots, $field );

if ( $format == "json" ) {
    echo json_encode( $response );
} else {
    echo implode( " ", $response );
}