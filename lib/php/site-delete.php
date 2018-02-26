<?php

parse_str( implode( '&', $args ) );

// Build arguments
$arguments = array(
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
$output = shell_exec( 'captaincore snapshot ' . $install . ' --delete-after-snapshot --email=support@anchor.host > /dev/null 2>/dev/null &' );
