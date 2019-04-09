<?php

// Converts --arguments into $arguments
parse_str( implode( '&', $args ) );

$curl = curl_init( "$captaincore_gui/wp-json/captaincore/v1/client" );
curl_setopt( $curl, CURLOPT_RETURNTRANSFER, 1 );
$response = curl_exec( $curl );
$response = json_decode( $response, true );

echo trim( $response[ $field ] );
