<?php

$arguments = array(
	'post_type'      => 'captcore_website',
	'posts_per_page' => '-1',
);

$websites = get_posts( $arguments );

foreach ( $websites as $website ) {

	$provider = get_post_meta( $website->ID, 'provider', true );
	$address  = get_post_meta( $website->ID, 'address', true );

	if ( $provider == '' ) {

		if ( strpos( $address, '.kinsta.com' ) !== false ) {
			echo 'Assigning kinsta to provider for ' . get_the_title( $website->ID ) . "\n";
			update_post_meta( $website->ID, 'provider', 'kinsta' );
		}

		if ( strpos( $address, '.wpengine.com' ) !== false ) {
			echo 'Assigning wpengine to provider for ' . get_the_title( $website->ID ) . "\n";
			update_post_meta( $website->ID, 'provider', 'wpengine' );
		}
	}
}
