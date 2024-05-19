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
parse_str( implode( '&', $args ), $arguments );
$arguments    = (object) $arguments;
$directory    = $arguments->directory;
$site_id      = $arguments->site_id;
$environment  = $arguments->environment;
$status       = $arguments->status;

if ( ! is_file( "{$directory}list.json" ) ) {
    file_put_contents( "{$directory}list.json", '[]' );
}

$email_checks = json_decode( file_get_contents( "{$directory}list.json" ) );
$found        = false;
foreach ( $email_checks as $email_check ) {
    if ( $email_check->site_id == $site_id && $email_check->environment == $environment ) {
        $found                    = true;
        $email_check->status      = $status;
        $email_check->received_at = (string) time();
    }
}

if ( ! $found ) {
    $email_checks[] = (object) [
        'site_id'      => $site_id,
        'environment'  => $environment,
        'status'       => $status,
        'received_at'  => (string) time(),
    ];
}

file_put_contents( "{$directory}list.json", json_encode( $email_checks, JSON_PRETTY_PRINT ) );