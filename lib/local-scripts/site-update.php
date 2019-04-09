<?php

// Replaces dashes in keys with underscores
foreach($args as $index => $arg) {
	$split = strpos($arg, "=");
	if ( $split ) {
		$key = str_replace('-', '_', substr( $arg , 0, $split ) );
		$value = substr( $arg , $split, strlen( $arg ) );

		// Removes unnessary bash quotes
		$value = trim( $value,'"' ); 				// Remove last quote 
		$value = str_replace( '="', '=', $value );  // Remove quote right after equals

		$args[$index] = $key.$value;
	} else {
		$args[$index] = str_replace('-', '_', $arg);
	}

}

// Converts --arguments into $arguments
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

$arguments = array(
	'author'    	 => $captain_id,
	'post_type'      => 'captcore_website',
	'posts_per_page' => '1',
	'meta_query'     => array(
		array(
			'key'     => 'site_id',
			'value'   => $id,
			'compare' => '=',
		),
	),
);

// Check if site
$found_site = get_posts( $arguments );

if ( $found_site ) {

	$found_site_id = $found_site[0]->ID;
	
	$my_post = array(

		'ID'          => $found_site_id,
		'post_title'  => $domain,
		'post_type'   => 'captcore_website',
		'post_status' => 'publish',
		'post_author' => $captain_id,
		'meta_input'  => array(
			'site_id'						  => $id,
			'site'                            => $site,
			'provider'                        => $provider,
			'fathom'                          => $fathom,
			'address'                         => $address,
			'username'                        => $username,
			'password'                        => $password,
			'protocol'                        => $protocol,
			'port'                            => $port,
			'home_directory'                  => $home_directory,
			'database_username'               => $database_username,
			'database_password'               => $database_password,
			'updates_enabled'                 => $updates_enabled,
			'updates_exclude_themes'          => $updates_exclude_themes,
			'updates_exclude_plugins'         => $updates_exclude_plugins,
			'offload_enabled'                 => $offload_enabled,
			'offload_provider'                => $offload_provider,
			'offload_access_key'              => $offload_access_key,
			'offload_secret_key'              => $offload_secret_key,
			'offload_bucket'                  => $offload_bucket,
			'offload_path'                    => $offload_path,
			'site_staging'                    => $staging_site,
			'address_staging'                 => $staging_address,
			'username_staging'                => $staging_username,
			'password_staging'                => $staging_password,
			'protocol_staging'                => $staging_protocol,
			'port_staging'                    => $staging_port,
			'home_directory_staging'          => $staging_home_directory,
			'database_username_staging'       => $staging_database_username,
			'database_password_staging'       => $staging_database_password,
			'updates_enabled_staging'         => $staging_updates_enabled,
			'updates_exclude_themes_staging'  => $staging_updates_exclude_themes,
			'updates_exclude_plugins_staging' => $staging_updates_exclude_plugins,
			'offload_enabled_staging'         => $staging_offload_enabled,
			'offload_access_key_staging'      => $staging_offload_access_key,
			'offload_secret_key_staging'      => $staging_offload_secret_key,
			'offload_bucket_staging'          => $staging_offload_bucket,
			'offload_path_staging'            => $staging_offload_path,
			'preloadusers'                    => $preloadusers,
			'status'                          => 'active',
		),
	);

	echo "Site updated\n";

	$site = wp_update_post( $my_post, true );

} else {

	$my_post = array(

		'post_title'  => $domain,
		'post_type'   => 'captcore_website',
		'post_status' => 'publish',
		'post_author' => $captain_id,
		'meta_input'  => array(
			'site_id'						  => $id,
			'site'                            => $site,
			'provider'                        => $provider,
			'fathom'                          => $fathom,
			'address'                         => $address,
			'username'                        => $username,
			'password'                        => $password,
			'protocol'                        => $protocol,
			'port'                            => $port,
			'home_directory'                  => $home_directory,
			'database_username'               => $database_username,
			'database_password'               => $database_password,
			'updates_enabled'                 => $updates_enabled,
			'updates_exclude_themes'          => $updates_exclude_themes,
			'updates_exclude_plugins'         => $updates_exclude_plugins,
			'offload_enabled'                 => $offload_enabled,
			'offload_provider'                => $offload_provider,
			'offload_access_key'              => $offload_access_key,
			'offload_secret_key'              => $offload_secret_key,
			'offload_bucket'                  => $offload_bucket,
			'offload_path'                    => $offload_path,
			'site_staging'                    => $staging_site,
			'address_staging'                 => $staging_address,
			'username_staging'                => $staging_username,
			'password_staging'                => $staging_password,
			'protocol_staging'                => $staging_protocol,
			'port_staging'                    => $staging_port,
			'home_directory_staging'          => $staging_home_directory,
			'database_username_staging'       => $staging_database_username,
			'database_password_staging'       => $staging_database_password,
			'updates_enabled_staging'         => $updates_enabled,
			'updates_exclude_themes_staging'  => $updates_exclude_themes,
			'updates_exclude_plugins_staging' => $updates_exclude_plugins,
			'offload_enabled_staging'         => $offload_enabled_staging,
			'offload_access_key_staging'      => $offload_access_key_staging,
			'offload_secret_key_staging'      => $offload_secret_key_staging,
			'offload_bucket_staging'          => $offload_bucket_staging,
			'offload_path_staging'            => $offload_path_staging,
			'preloadusers'                    => $preloadusers,
			'status'                          => 'active',
		)
	);

	$result = wp_insert_post( $my_post, true );
	echo "Site added\n";
	if ( is_wp_error( $result ) ) {
		$error_string = $result->get_error_message();
		echo $error_string;
	}
}
