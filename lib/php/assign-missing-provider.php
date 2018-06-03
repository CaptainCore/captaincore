<?php

$arguments = array(
	'post_type'      => 'captcore_website',
	'posts_per_page' => '-1',
);

$websites = get_posts( $arguments );

foreach ( $websites as $website ) {

	$provider = get_field( 'provider', $website->ID );
	$address  = get_field( 'address', $website->ID );

	if ( $provider == '' ) {

		if ( strpos( $address, '.kinsta.com' ) !== false ) {
			echo 'Assigning kinsta to provider for ' . get_the_title( $website->ID ) . "\n";
			update_field( 'provider', 'kinsta', $website->ID );
		}

		if ( strpos( $address, '.wpengine.com' ) !== false ) {
			echo 'Assigning wpengine to provider for ' . get_the_title( $website->ID ) . "\n";
			update_field( 'provider', 'wpengine', $website->ID );
		}
	}
}
