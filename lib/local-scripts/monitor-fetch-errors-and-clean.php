<?php

$output           = array();
$urls_with_errors = array();
$contents         = file_get_contents( $args[0] );
$lines            = explode( "\n", $contents );

foreach ( $lines as $line ) {
	$json = json_decode( $line );

	// Check if JSON valid
	if ( json_last_error() !== JSON_ERROR_NONE ) {
		continue;
	}

	$http_code  = $json->http_code;
	$url        = $json->url;
	$html_valid = $json->html_valid;

	// Check if HTML is valid
	if ( $html_valid == 'false' ) {
		$urls_with_errors[] = $url;
		continue;
	}

	// Check if healthy
	if ( $json->http_code == '200' ) {
    $output[] = $line;
		continue;
	}

	// Check for redirects
	if ( $json->http_code == '301' ) {
    $output[] = $line;
		continue;
	}

	// Append error to errors for email purposes
  $urls_with_errors[] = $url;
}

// Update log file without errors
$contents_updated = implode( "\n", $output );
file_put_contents( $args[0], $contents_updated );

// Return URLs with errors
echo implode( " ", $urls_with_errors );
