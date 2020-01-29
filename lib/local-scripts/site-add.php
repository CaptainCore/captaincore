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

if ( $format == 'base64' ) {
	$site         = json_decode( base64_decode( $details ) ) ;
	$site_check   = ( new CaptainCore\Sites )->get( $site->site_id );
	$environments = $site->environments;
	unset( $site->environments );
	if ( empty( $site_check ) ) {
		// Insert new site
		( new CaptainCore\Sites )->insert( (array) $site );
	} else {
		// update new site
		( new CaptainCore\Sites )->update( (array) $site, [ "site_id" => $site->site_id ] );
	}
	foreach ( $environments as $environment ) {
		$environment_id    = $environment->environment_id;
		$environment_check = ( new CaptainCore\Environments )->get( $environment_id );
		// Insert new environment
		if ( empty( $environment_check ) ) {
			( new CaptainCore\Environments )->insert( (array) $environment );
			continue;
		}
		// Update existing environment
		( new CaptainCore\Environments )->update( (array) $environment, [ "environment_id" => $environment_id ] );
	}

}