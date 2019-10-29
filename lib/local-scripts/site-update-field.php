<?php

$field     = $args[1];
$new_value = $args[2];

print_r(  $args );

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

$arguments = array(
	'author'    	 => $captain_id,
	'post_type'      => 'captcore_website',
	'posts_per_page' => '1',
	'meta_query'     => array(
		array(
			'key'     => 'site_id',
			'value'   => $site_id,
			'compare' => '=',
		),
	),
);

// Check if site
$found_site = get_posts( $arguments );

if ( $found_site ) {

	$found_site_id = $found_site[0]->ID;
	
	$site_update = [
		'ID'          => $found_site_id,
		'post_author' => $captain_id,
		'meta_input'  => [ $field => $new_value ],
	];

	print_r( $site_update );

	echo "Site updated\n";

	$site = wp_update_post( $site_update, true );

}
