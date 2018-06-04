<?php

// Converts arguments into variables
parse_str( implode( '&', $args ) );

// WP_Query arguments
$arguments = array(
	'post_type'      => array( 'captcore_website' ),
	'posts_per_page' => '1',
	'fields'         => 'ids',
	'meta_query'     => array(
		'relation' => 'AND',
		array(
			'key'     => 'site',
			'value'   => $site,
			'compare' => '=',
		),
	),
);

// If provider specified
if ( $provider ) {
	$arguments['meta_query'][] = array(
		'key'     => 'provider',
		'value'   => $provider,
		'compare' => '=',
	);
}

// The Query
$site_ids = get_posts( $arguments );

// Bash output
foreach ( $site_ids as $site_id ) {
	$site   = get_post( $site_id );
	$fields = array( 'ID', 'domain', 'site', 'provider', 'address', 'username', 'protocol', 'port', 'homedir', 'database_username', 'database_password', 'site_staging', 'address_staging', 'username_staging', 'protocol_staging', 'port_staging', 'homedir_staging', 'database_username_staging', 'database_password_staging', 's3accesskey', 's3secretkey', 's3bucket', 's3path', 'preloadusers', 'home_url' );
	if ( $field ) {
		$fields = array( $field );
	}
	$title = $site->post_title;

	$json = '[{';

	$bash = '';

	foreach ( $fields as $f ) {
		if ( $f == 'ID' ) {
			$value = get_post_meta( $site_id, $f, true );
			$bash .= "site_id=$site_id\n";
			$json .= "\"ID\":\"$site_id\",";
		} elseif ( $f == 'domain' ) {
			$bash .= "domain=$title\n";
			$json .= "\"domain\":\"$title\",";
		} else {
			$value = get_post_meta( $site_id, $f, true );
			$bash .= "$f=$value\n";
			$json .= "\"$f\":\"$value\",";
		}
	}
	$json  = substr( $json, 0, -1 );
	$json .= '}]';

}
if ( $field ) {
	if ( $field == 'ID' ) {
		$value = get_post_meta( $site_id, $f, true );
		$json = $site_id;
	} elseif ( $field == 'domain' ) {
		$json = $title;
	} else {
		$value = get_post_meta( $site_id, $field, true );
		$json = $value;
	}
}

if ( $format == 'bash' ) {
		echo $bash;
} else {
		echo $json;
}
