<?php

// Converts arguments --staging --all --plugin=woocommerce --theme=sitename1 into $staging $all
parse_str( implode( '&', $args ) );

// Decodes passwords
$password         = base64_decode( urldecode( $password ) );
$staging_password = base64_decode( urldecode( $staging_password ) );

// Detect if provider passed into <site>
if ( strpos( $site, '@' ) !== false ) {
	$split    = explode( '@', $site, 2 );
	$site     = $split[0];
	$provider = $split[1];
}

// Check if site
$found_site = get_post( $id );

if ( $found_site ) {

	$my_post = array(

		'ID'          => $id,
		'post_title'  => $domain,
		'post_type'   => 'captcore_website',
		'post_status' => 'publish',
		'post_author' => '1',
		'meta_input'  => array(
			'site'                      => $site,
			'provider'                  => $provider,
			'fathom'                    => $fathom,
			'address'                   => $address,
			'username'                  => $username,
			'password'                  => $password,
			'protocol'                  => $protocol,
			'port'                      => $port,
			'homedir'                   => $homedir,
			'database_username'         => $database_username,
			'database_password'         => $database_password,
			'site_staging'              => $staging_site,
			'address_staging'           => $staging_address,
			'username_staging'          => $staging_username,
			'password_staging'          => $staging_password,
			'protocol_staging'          => $staging_protocol,
			'port_staging'              => $staging_port,
			'homedir_staging'           => $staging_homedir,
			'database_username_staging' => $staging_database_username,
			'database_password_staging' => $staging_database_password,
			'updates_enabled'           => $updates_enabled,
			'exclude_themes'            => $exclude_themes,
			'exclude_plugins'           => $exclude_plugins,
			'preloadusers'              => $preloadusers,
			's3accesskey '              => $s3accesskey,
			's3secretkey '              => $s3secretkey,
			's3bucket'                  => $s3bucket,
			's3path '                   => $s3path,
			'status'                    => 'active',

		),
	);

	echo "Site updated\n";

	$site = wp_update_post( $my_post, true );

} else {

	$my_post = array(

		'import_id'   => intval( $id ),
		'post_title'  => $domain,
		'post_type'   => 'captcore_website',
		'post_status' => 'publish',
		'post_author' => '1',
		'meta_input'  => array(
			'site'                      => $site,
			'provider'                  => $provider,
			'fathom'                    => $fathom,
			'address'                   => $address,
			'username'                  => $username,
			'password'                  => $password,
			'protocol'                  => $protocol,
			'port'                      => $port,
			'homedir'                   => $homedir,
			'database_username'         => $database_username,
			'database_password'         => $database_password,
			'site_staging'              => $staging_site,
			'address_staging'           => $staging_address,
			'username_staging'          => $staging_username,
			'password_staging'          => $staging_password,
			'protocol_staging'          => $staging_protocol,
			'port_staging'              => $staging_port,
			'homedir_staging'           => $staging_homedir,
			'database_username_staging' => $staging_database_username,
			'database_password_staging' => $staging_database_password,
			'updates_enabled'           => $updates_enabled,
			'exclude_themes'            => $exclude_themes,
			'exclude_plugins'           => $exclude_plugins,
			'preloadusers'              => $preloadusers,
			's3accesskey '              => $s3accesskey,
			's3secretkey '              => $s3secretkey,
			's3bucket'                  => $s3bucket,
			's3path '                   => $s3path,
			'status'                    => 'active',

		),
	);

	$result = wp_insert_post( $my_post, true );
	echo "Site added\n";
	if ( is_wp_error( $result ) ) {
		$error_string = $result->get_error_message();
		echo $error_string;
	}
}
