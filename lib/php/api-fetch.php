<?php

// Converts arguments --staging --all --plugin=woocommerce --theme=anchorhost into $staging $all
parse_str( implode( '&', $args ) );

$curl = curl_init( $captaincore_wordpress_site . "/wp-json/captaincore/v1/client" );
curl_setopt( $curl, CURLOPT_RETURNTRANSFER, 1 );
$response = curl_exec( $curl );
$response = json_decode( $response, true );

echo trim( $response[ $field ] );
