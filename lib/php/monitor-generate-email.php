<?php

$errors = array();
$warnings = array();

$contents = file_get_contents( $args[0] );

$lines = explode("\n", $contents);

foreach ($lines as $line) {
  $json = json_decode( $line );

	if (json_last_error() !== JSON_ERROR_NONE ) {
    # JSON is valid
		continue;
	}

	$http_code = $json->http_code;
	$url = $json->url;

	# Just became healthy. Update status and mark new health
	if ( $json->http_code == "200" ) {

		continue;
	}

	# Handle redirects
	if ( $json->http_code == "301" ) {
		$warnings[] = "Response code $http_code for $url\n";
		continue;
	}

	# Append error to errors for email purposes
	$errors[] = "Response code $http_code for $url\n";
}

if ( count($errors) > 0 ) {

	$html = "<strong>Errors</strong><br /><br />";

	foreach ($errors as $error) {
		$html .= trim($error) . "<br />\n";
	};

	$html .= "<br /><strong>Warnings</strong><br /><br />";

	foreach ($warnings as $warning) {
		$html .= trim($warning) . "<br />\n";
	};

	echo $html;

}
