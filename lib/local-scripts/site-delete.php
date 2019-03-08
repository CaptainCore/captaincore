<?php

parse_str( implode( '&', $args ) );

// Loads CLI configs
$file = $_SERVER['HOME'] . '/.captaincore-cli/config';
if ( file_exists( $file ) ) {

	$file = file_get_contents( $file );
	// Matches config keys and values
	$pattern = '/(.+)=\"(.+)\"/';
	preg_match_all( $pattern, $file, $matches );

}

// Fetches from CLI configs
$captaincore_admin_email_key = array_search( 'captaincore_admin_email', $matches[1] );
$captaincore_admin_email    = $matches[2][ $captaincore_admin_email_key ];

if ( $captain_id == "" ) {
	$captain_id = 1;
}

// Build arguments
$arguments = array(
	'author'    	 => $captain_id,
	'post_type'      => 'captcore_website',
	'posts_per_page' => '-1',
	'fields'         => 'ids',
	'post__in'       => array( $id ),
	'meta_query'     => array(
		'relation' => 'AND',
		array(
			'key'     => 'status', // name of custom field
			'value'   => 'active', // matches exaclty "123", not just 123. This prevents a match for "1234"
			'compare' => '=',
		),
	),
);

// Check if site
$found_site = get_posts( $arguments );

if ( $found_site ) {

	$my_post = array(

		'ID'          => $id,
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
$output = shell_exec( "captaincore snapshot $site --delete-after-snapshot --email=$captaincore_admin_email > /dev/null 2>/dev/null &" );
