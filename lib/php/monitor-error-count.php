<?php

$errors = array();

$contents = file_get_contents( $args[0] );

$lines = explode("\n", $contents);

foreach ($lines as $line) {
  $json = json_decode( $line );

	if (json_last_error() !== JSON_ERROR_NONE ) {
    # JSON is valid
		continue;
	}

	# Just became healthy. Update status and mark new health
	if ( $json->http_code == "200" ) {

		continue;
	}

	# Handle redirects
	if ( $json->http_code == "301" ) {

		continue;
	}

	# Append error to errors for email purposes
	$errors[] = "Response code $http_code for $url\n";
}

echo count($errors);
