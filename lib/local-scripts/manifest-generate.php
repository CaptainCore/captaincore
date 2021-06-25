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

$json = $_SERVER['HOME'] . "/.captaincore/config.json";
$config_data = json_decode ( file_get_contents( $json ) );
$system = $config_data[0]->system;

foreach($config_data as $config) {
	if ( isset( $config->captain_id ) and $config->captain_id == $captain_id ) {
		$configuration = $config;
		break;
	}
}

$manifest_path = $system->path;
if ( $system->captaincore_fleet == "true" ) {
	$manifest_path = "{$manifest_path}/{$captain_id}";
}

$sites              = ( new CaptainCore\Sites )->list_details();
$quicksave_storage  = 0;
$quicksave_count    = 0;
$total_site_storage = 0;
$total_storage      = 0; 

foreach( $sites as $site ) {
	if ( $site->storage_raw == "" ) { 
		$site->storage_raw = 0; 
	}
	if ( $site->quicksaves_usage == "" ) {
		$site->quicksaves_usage = [];
	}
	$quicksave_count    = $quicksave_count + array_sum( array_column ( $site->quicksaves_usage, "count" ) );
	$quicksave_storage  = $quicksave_storage + array_sum( array_column ( $site->quicksaves_usage, "storage" ) );
	$total_site_storage = $total_site_storage + $site->storage_raw;
	$total_storage      = $total_storage + $site->storage_raw + array_sum( array_column ( $site->quicksaves_usage, "storage" ) );
}

$manifest = [
	'sites'      => [
		'count'   => count( $sites ),
		'storage' => $total_site_storage
	],
	'quicksaves' => [
		'count'   => $quicksave_count,
		'storage' => $quicksave_storage
	],
	'storage'    => $total_storage
];

$results = json_encode( $manifest, JSON_PRETTY_PRINT );
echo $results;

file_put_contents( "{$manifest_path}/manifest.json", $results  );