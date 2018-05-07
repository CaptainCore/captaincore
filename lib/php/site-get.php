<?php

$website_id = $args[0];
$website    = get_post( $website_id );
$fields     = array( 'site', 'address', 'username', 'protocol', 'port', 'homedir', 'database_username', 'database_password', 'site_staging', 'address_staging', 'username_staging', 'protocol_staging', 'port_staging', 'homedir_staging', 'database_username_staging', 'database_password_staging', 's3accesskey', 's3secretkey', 's3bucket', 's3path', 'preloadusers', 'home_url' );
$title      = $website->post_title;

echo "site_id=$website_id\n";
echo "domain=$title\n";
foreach ( $fields as $field ) {
	$value = get_post_meta( $website_id, $field, true );
	echo "$field=$value\n";
}
