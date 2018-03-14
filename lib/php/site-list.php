<?php

// Replaces dashes in keys with underscores
foreach($args as $index => $arg) {
	$split = strpos($arg, "=");
	$key = str_replace('-', '_', substr( $arg , 0, $split ) );
	$value = substr( $arg , $split, strlen( $arg ) );
	$args[$index] = $key.$value;
}

// Converts arguments --staging --all into $staging $all
parse_str( implode( '&', $args ) );

if ( isset( $all ) ) {
	echo 'all';
}

$arguments = array(
	'post_type'      => 'captcore_website',
	'posts_per_page' => '-1',
	'fields'         => 'ids',
	'meta_query'     => array(
		'relation' => 'AND',
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

if ( $filter ) {

	if ( $filter and $filter_version and $filter_status and $filter_name ) {

		$pattern   = '{"name":"'.$filter_name.'","title":"[^"]*","status":"'.$filter_status.'","version":"'.$filter_version.'"}';
		$arguments['meta_query'][] = array(
			'key'     => $filter .'s', // name of custom field
			'value'   => $pattern,
			'compare' => 'REGEXP',
		);

	} elseif ( $filter and $filter_status and $filter_name ) {

		$pattern   = '{"name":"'.$filter_name.'","title":"[^"]*","status":"'.$filter_status.'","version":"[^"]*"}';
		$arguments['meta_query'][] = array(
			'key'     => $filter .'s', // name of custom field
			'value'   => $pattern,
			'compare' => 'REGEXP',
		);

	} elseif ( $filter and $filter_version and $filter_name ) {

		$pattern   = '{"name":"'.$filter_name.'","title":"[^"]*","status":"[^"]*","version":"'.$filter_version.'"}';
		$arguments['meta_query'][] = array(
			'key'     => $filter .'s', // name of custom field
			'value'   => $pattern,
			'compare' => 'REGEXP',
		);

	} elseif ( $filter and $filter_name ) {

		if ( $filter == "core" ) {
			$filter_key = "core";
		} else {
			// Pluralize
			$filter_key = $filter . "s";
		}

		$arguments['meta_query'][] = array(
			'key'     => $filter_key, // name of custom field
			'value'   => '"name":"' . $filter_name . '"', // matches exaclty "123", not just 123. This prevents a match for "1234"
			'compare' => 'like',
		);

	} elseif ( $filter and $filter_version ) {
		$arguments['meta_query']['relation'] = 'OR';
		$pattern   = '{"name":"[^"]*","title":"[^"]*","status":"[^"]*","version":"'.$filter_version.'"}';
		$arguments['meta_query'][] = array(
			'key'     => 'plugins', // name of custom field
			'value'   => $pattern,
			'compare' => 'REGEXP',
		);
		$arguments['meta_query'][] = array(
			'key'     => 'themes', // name of custom field
			'value'   => $pattern,
			'compare' => 'REGEXP',
		);
		$arguments['meta_query'][] = array(
			'key'     => 'core', // name of custom field
			'value'   => $filter_version, // matches exaclty "123", not just 123. This prevents a match for "1234"
			'compare' => 'like',
		);
	}

}

$websites = get_posts( $arguments );

$results = array();

foreach ( $websites as $website_id ) {

	$site = get_post_meta( $website_id, 'install', true );

	if ( $field ) {
		if ( $field == 'ids' ) {
			$site = $website_id;
		} elseif ( $field == 'domain' ) {
			$site = get_the_title( $website_id );
		} else {
			$site = get_post_meta( $website_id, $field, true );
		}
	}

	if ( isset( $staging ) ) {
		$results[] = $site . '-staging';
	} elseif ( isset( $all ) ) {
		$results[] = $site;
		$results[] = $site . '-staging';
	} else {
		$results[] = $site;
	}
}

echo implode( ' ', $results );
