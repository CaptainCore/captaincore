<?php

// Converts arguments --staging --all --plugin=woocommerce --plugin_status=active --theme=anchorhost into $staging $all
parse_str( str_replace( '-', '_', implode( '&', $args ) ) );

$arguments = array(
	'post_type'      => 'captcore_website',
	'posts_per_page' => '-1',
	'fields'         => 'ids',
	's'              => $search,
	'meta_query'     => array(
		'relation' => 'and',
		array(
			'key'     => 'status', // name of custom field
			'value'   => 'active', // matches exaclty "123", not just 123. This prevents a match for "1234"
			'compare' => '=',
		),
		array(
			'key'     => 'install', // name of custom field
			'value'   => '',
			'compare' => '!=',
		),
	),
);

$websites = get_posts( $arguments );

$results = array();

foreach ( $websites as $website_id ) {

	$site = get_post_meta( $website_id, 'install', true );
	$results[] = $site;

}

echo implode( ' ', $results );
