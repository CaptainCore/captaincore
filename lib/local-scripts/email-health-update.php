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
$file         = $arguments->file;
$site_id      = $arguments->site_id;
$environment  = $arguments->environment;
$status       = $arguments->status;

$email_checks = json_decode( file_get_contents( $file ) );
foreach ( $email_checks as $email_check ) {
    if ( $email_check->site_id == $site_id && $email_check->environment == $environment ) {
        $email_check->status      = $status;
        $email_check->received_at = (string) time();
    }
}

file_put_contents( $file, json_encode( $email_checks, JSON_PRETTY_PRINT ) );