<?php

$website_id = $args[0];
$website    = get_post( $website_id );
$fields     = array( 'install', 'address', 'username', 'password', 'port', 'homedir', 'database_username', 'database_password', 'install_staging', 'address_staging', 'username_staging', 'password_staging', 'protocol_staging', 'port_staging', 'homedir_staging', 'database_username_staging', 'database_password_staging', 's3accesskey', 's3secretkey', 's3bucket', 's3path' );
$title      = $website->post_title;

echo "domain=$title\n";
foreach ( $fields as $field ) {
	$value = get_post_meta( $website_id, $field, true );
	echo "$field=$value\n";
}
