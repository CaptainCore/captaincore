<?php

$arguments = array(
	'post_type'      => 'captcore_quicksave',
	'posts_per_page' => '-1',
	'fields'         => 'ids',
);

$quicksaves = get_posts( $arguments );

foreach ( $quicksaves as $quicksave_id ) {

	$git_commmit = get_field( 'git_commit', $quicksave_id );
	$website     = get_field( 'website', $quicksave_id );

	$args = array(
		'post_type'      => 'captcore_quicksave',
		'posts_per_page' => '-1',
		'fields'         => 'ids',
		'meta_query'     => array(
			'relation' => 'AND',
			array(
				'key'     => 'git_commit', // name of custom field
				'value'   => $git_commmit, // matches exactly "123", not just 123. This prevents a match for "1234"
				'compare' => '=',
			),
			array(
				'key'     => 'website', // name of custom field
				'value'   => '"' . $website[0] . '"', // matches exactly "123", not just 123. This prevents a match for "1234"
				'compare' => 'LIKE',
			),
		),
	);

	$quicksave_duplicate_search = get_posts( $args );
	asort( $quicksave_duplicate_search );

	if ( count( $quicksave_duplicate_search ) > 1 ) {
		echo 'Found duplicate quicksaves (' . count( $quicksave_duplicate_search ) . " for git_commit $git_commmit) \n";
		unset( $quicksave_duplicate_search[0] ); // Remove first to prevent deleting the master
		foreach ( $quicksave_duplicate_search as $quicksave_duplicate_id ) {
			// Remove duplicates
			wp_delete_post( $quicksave_duplicate_id, true );
			echo "Removed quicksave duplicate $quicksave_duplicate_id \n";
		}
	}
}

// print_r($quicksaves);
