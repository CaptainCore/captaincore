<?php

// Replaces dashes in keys with underscores
foreach($args as $index => $arg) {
	$split = strpos($arg, "=");
	if ( $split ) {
		$key = str_replace('-', '_', substr( $arg , 0, $split ) );
		$value = substr( $arg , $split, strlen( $arg ) );

		// Removes unnecessary bash quotes
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

$lookup  = ( new CaptainCore\Sites )->where( [ "site" => $site ] );
if ( $provider ) {
	$lookup  = ( new CaptainCore\Sites )->where( [ "site" => $site, "provider" => $provider ] );
}

// Error if site not found
if ( count( $lookup ) == 0 ) {
	return "";
}

// Fetch site
$site    = ( new CaptainCore\Site( $lookup[0]->site_id ) )->get();

// Set environment if not defined
if ( $environment == "" ) {
	$environment = "Production";
}

$environment_key = array_search( ucfirst($environment), array_column( $site->environments, 'environment' ) );

$address                 = $site->environments[$environment_key]->address;
$username                = $site->environments[$environment_key]->username;
$password                = $site->environments[$environment_key]->password;
$protocol                = $site->environments[$environment_key]->protocol;
$port                    = $site->environments[$environment_key]->port;
$home_directory          = $site->environments[$environment_key]->home_directory;
$database_username       = $site->environments[$environment_key]->database_username;
$database_password       = $site->environments[$environment_key]->database_password;
$capture_pages           = $site->environments[$environment_key]->capture_pages;
$fathom                  = $site->environments[$environment_key]->fathom;
$offload_enabled         = $site->environments[$environment_key]->offload_enabled;
$offload_provider        = $site->environments[$environment_key]->offload_provider;
$offload_access_key      = $site->environments[$environment_key]->offload_access_key;
$offload_secret_key      = $site->environments[$environment_key]->offload_secret_key;
$offload_bucket          = $site->environments[$environment_key]->offload_bucket;
$offload_path            = $site->environments[$environment_key]->offload_path;
$home_url                = $site->environments[$environment_key]->home_url;
$updates_enabled         = $site->environments[$environment_key]->updates_enabled;
$updates_exclude_themes  = $site->environments[$environment_key]->updates_exclude_themes;
$updates_exclude_plugins = $site->environments[$environment_key]->updates_exclude_plugins;

$array = [
	"site_id"                 => $site->site_id,
	"site"                    => $site->site,
	"status"                  => $site->status,
	"provider"                => $site->provider,
	"key"                     => $site->key,
	"domain"                  => $site->name,
	"home_url"                => $home_url,
	"defaults"                => json_encode( $site->account["defaults"] ),
	"fathom"                  => json_encode( $fathom ),
	"capture_pages"           => $capture_pages,
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
];

if ( $format == 'bash' and $capture_pages != "" ) {
	// Return as CSV
	$capture_pages = implode(",", array_column( json_decode( $capture_pages ), "page" ) );
}

$default_users = json_encode ( $site->account["defaults"]->users );

$bash = "site_id={$site->site_id}
domain={$site->name}
key={$site->key}
fathom=$fathom
capture_pages=$capture_pages
site={$site->site}
status={$site->status}
provider={$site->provider}
default_users=$default_users
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

if ( $field ) {
	echo $array[$field];
	return true;
}

if ( $format == 'bash' ) {
	echo $bash;
}

if ( $format == 'json' ) {
	echo json_encode( $array, JSON_PRETTY_PRINT );
}
