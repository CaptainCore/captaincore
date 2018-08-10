<?php

$errors = array();

$contents = file_get_contents( $args[0] );

$lines = explode("\n", $contents);

foreach ($lines as $line) {
  $json = json_decode( $line );

	# Check if JSON valid
  if (json_last_error() !== JSON_ERROR_NONE ) {
    continue;
  }

  $http_code = $json->http_code;
  $url = $json->url;
  $html_valid = $json->html_valid;

  # Check if HTML is valid
  if ( $html_valid == "false" ) {
    $errors[] = "Response code $http_code for $url html is invalid\n";
    continue;
  }

  # Check if healthy
  if ( $json->http_code == "200" ) {

    continue;
  }

  # Check for redirects
  if ( $json->http_code == "301" ) {
    continue;
  }

  # Append error to errors for email purposes
  $errors[] = "Response code $http_code for $url\n";
}

echo count($errors);
