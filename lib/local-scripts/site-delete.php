<?php

// Replaces dashes in keys with underscores
foreach($args as $index => $arg) {
	$split = strpos($arg, "=");
	if ( $split ) {
		$key = str_replace('-', '_', substr( $arg , 0, $split ) );
		$value = substr( $arg , $split, strlen( $arg ) );

		// Removes unnessary bash quotes
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

// Build arguments
$arguments = array(
	'author'    	 => $captain_id,
	'post_type'      => 'captcore_website',
	'posts_per_page' => '1',
	'meta_query'     => array(
		array(
			'key'     => 'site_id',
			'value'   => $id,
			'compare' => '=',
		),
	),
);
// Check if site
$found_site = get_posts( $arguments );

if ( $found_site ) {

	$found_site_id = $found_site[0]->ID;

	$my_post = array(

		'ID'          => $found_site_id,
		'post_status' => 'publish',
		'meta_input'  => array(
			'status' => 'closed',
		),
	);
	echo "Site removed\n";

	wp_update_post( $my_post );

} else {

	echo "Site not found\n";

}

// Make final snapshot then remove local files
$output = shell_exec( "captaincore snapshot $site --delete-after-snapshot --email=$captaincore_admin_email --captain_id=$captain_id > /dev/null 2>/dev/null &" );
