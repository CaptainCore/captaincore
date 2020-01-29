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

$lookup  = ( new CaptainCore\Sites )->where( [ "site" => $site ] );
if ( $provider ) {
	$lookup  = ( new CaptainCore\Sites )->where( [ "site" => $site, "provider" => $provider ] );
}

// Error if site not found
if ( count( $lookup ) == 0 ) {
	return "";
}

// Fetch site
$environment_id = ( new CaptainCore\Site( $lookup[0]->site_id ) )->fetch_environment_id( $environment );

$code = str_replace( "'", "", $code );

// Update current environment with new data.
echo "Updating environment $environment_id";
( new CaptainCore\Environments )->update( [ "fathom" => $code ], [ "environment_id" => $environment_id ] );