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

$json = $_SERVER['HOME'] . "/.captaincore-cli/config.json";
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

$arguments = array(
	'author'		 => $captain_id,
	'post_type'      => 'captcore_website',
	'posts_per_page' => '-1',
	'fields'         => 'ids',
	'meta_query'     => array(
		'relation' => 'AND',
		array(
			'key'     => 'status', // name of custom field
			'value'   => 'active', // matches exactly "123", not just 123. This prevents a match for "1234"
			'compare' => '=',
		),
		array(
			'key'     => 'site', // name of custom field
			'value'   => '',
			'compare' => '!=',
		),
	),
);

$websites = get_posts( $arguments );
$quicksave_storage = 0;
$quicksave_count = 0;
$total_site_storage = 0;
$total_storage = 0; 

foreach( $websites as $website ) {
	$site_storage     = get_post_meta( $website, "storage", true );
	$quicksaves_usage = json_decode( get_post_meta( $website, "quicksaves_usage", true ) );

	if ( $site_storage == "" ) { 
		$site_storage = 0; 
	}
	if ( $quicksaves_usage == "" ) {
		$quicksaves_usage = json_decode( '{"count":"0","storage":"0"}' );
	}
	$quicksave_count = $quicksave_count + $quicksaves_usage->count;
	$quicksave_storage = $quicksave_storage + $quicksaves_usage->storage;
	$total_site_storage = $total_site_storage + $site_storage;
	$total_storage = $total_storage + $site_storage + $quicksaves_usage->storage;
}

$manifest = array(
	'sites'      => array( 
		'count'   => count( $websites ),
		'storage' => $total_site_storage
	),
	'quicksaves' => array(
		'count'   => $quicksave_count,
		'storage' => $quicksave_storage
	),
	'storage'    => $total_storage
);

$results = json_encode( $manifest, JSON_PRETTY_PRINT );
echo $results;

file_put_contents( "{$manifest_path}/manifest.json", $results  );