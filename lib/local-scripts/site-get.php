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

// Assign default format to JSON
if ( $format == "" ) {
	$format = "json";
}

// WP_Query arguments
$arguments = array(
	'author'    	 => $captain_id,
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

	// Bail if enviroment not set
	if ( ! $environment == "production" or ! $environment == "staging" ) {
		return;
	}

	$title        = get_the_title( $site_id );
	$site         = get_post_meta( $site_id, "site", true );
	$id           = get_post_meta( $site_id, "site_id", true );
	$provider     = get_post_meta( $site_id, "provider", true );
	$preloadusers = get_post_meta( $site_id, "preloadusers", true );
	$status       = get_post_meta( $site_id, "status", true );

	if ( $environment == "production" ) {
		$address                 = get_post_meta( $site_id, "address", true );
		$username                = get_post_meta( $site_id, "username", true );
		$password                = get_post_meta( $site_id, "password", true );
		$protocol                = get_post_meta( $site_id, "protocol", true );
		$port                    = get_post_meta( $site_id, "port", true );
		$home_directory          = get_post_meta( $site_id, "home_directory", true );
		$database_username       = get_post_meta( $site_id, "database_username", true );
		$database_password       = get_post_meta( $site_id, "database_password", true );
		$fathom                  = get_post_meta( $site_id, "fathom", true );
		$offload_enabled         = get_post_meta( $site_id, "offload_enabled", true );
		$offload_provider        = get_post_meta( $site_id, "offload_provider", true );
		$offload_access_key      = get_post_meta( $site_id, "offload_access_key", true );
		$offload_secret_key      = get_post_meta( $site_id, "offload_secret_key", true );
		$offload_bucket          = get_post_meta( $site_id, "offload_bucket", true );
		$offload_path            = get_post_meta( $site_id, "offload_path", true );
		$home_url                = get_post_meta( $site_id, "home_url", true );
		$updates_enabled         = get_post_meta( $site_id, "updates_enabled", true );
		$updates_exclude_themes  = get_post_meta( $site_id, "updates_exclude_themes", true );
		$updates_exclude_plugins = get_post_meta( $site_id, "updates_exclude_plugins", true );
	}

	if ( $environment == "staging" ) {
		$address                 = get_post_meta( $site_id, "address_staging", true );
		$username                = get_post_meta( $site_id, "username_staging", true );
		$password                = get_post_meta( $site_id, "password_staging", true );
		$protocol                = get_post_meta( $site_id, "protocol_staging", true );
		$port                    = get_post_meta( $site_id, "port_staging", true );
		$home_directory          = get_post_meta( $site_id, "home_directory_staging", true );
		$database_username       = get_post_meta( $site_id, "database_username_staging", true );
		$database_password       = get_post_meta( $site_id, "database_password_staging", true );
		$fathom                  = get_post_meta( $site_id, "fathom_staging", true );
		$offload_enabled         = get_post_meta( $site_id, "offload_enabled_staging", true );
		$offload_provider        = get_post_meta( $site_id, "offload_provider_staging", true );
		$offload_access_key      = get_post_meta( $site_id, "offload_access_key_staging", true );
		$offload_secret_key      = get_post_meta( $site_id, "offload_secret_key_staging", true );
		$offload_bucket          = get_post_meta( $site_id, "offload_bucket_staging", true );
		$offload_path            = get_post_meta( $site_id, "offload_path_staging", true );
		$home_url                = get_post_meta( $site_id, "home_url_staging", true );
		$updates_enabled         = get_post_meta( $site_id, "updates_enabled_staging", true );
		$updates_exclude_themes  = get_post_meta( $site_id, "updates_exclude_themes_staging", true );
		$updates_exclude_plugins = get_post_meta( $site_id, "updates_exclude_plugins_staging", true );
	}

	$array = array(
		"ID"                      => $site_id,
		"site_id"                 => $id,
		"site"                    => $site,
		"status"				  => $status,
		"provider"                => $provider,
		"preloadusers"            => $preloadusers,
		"home_url"                => $home_url,
		"domain"                  => $title,
		"fathom"                  => $fathom,
		'address'                 => $address,
		'username'                => $username,
		'password'                => $password,
		'protocol'                => $protocol,
		'port'                    => $port,
		'home_directory'          => $home_directory,
		'database_username'       => $database_username,
		'database_password'       => $database_password,
		'updates_enabled'         => $updates_enabled,
		'updates_exclude_themes'  => $updates_exclude_themes,
		'updates_exclude_plugins' => $updates_exclude_plugins,
		'offload_enabled'         => $offload_enabled,
		'offload_provider'        => $offload_provider,
		'offload_access_key'      => $offload_access_key,
		'offload_secret_key'      => $offload_secret_key,
		'offload_bucket'          => $offload_bucket,
		'offload_path'            => $offload_path,
	);

	$bash = "id=$site_id
site_id=$id
domain=$title
fathom=$fathom
site=$site
site_status=$status
provider=$provider
preloadusers=$preloadusers
home_url=$home_url
address=$address
username=$username
protocol=$protocol
port=$port
home_directory=$home_directory
database_username=$database_username
database_password=$database_password
updates_enabled=$updates_enabled
updates_exclude_themes=$updates_exclude_themes
updates_exclude_plugins=$updates_exclude_plugins
offload_enabled=$offload_enabled
offload_provider=$offload_provider
offload_access_key=$offload_access_key
offload_secret_key=$offload_secret_key
offload_bucket=$offload_bucket
offload_path=$offload_path";

}
if ( $field ) {
	echo $array[$field];
	return true;
}

if ( $format == 'bash' ) {
	echo $bash;
} 

if ( $format == 'json' ) {
	echo json_encode($array, JSON_PRETTY_PRINT);
}
