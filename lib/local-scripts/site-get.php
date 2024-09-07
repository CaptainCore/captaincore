<?php

$environment = "";

// Converts --arguments into $arguments
parse_str( implode( '&', $args ), $args );

$site   = array_keys( $args )[0];
$format = empty( $args["format"] ) ? "" : $args["format"];

if( strpos( $site, "-" ) !== false ) {
	$split       = explode( "-", $site );
	$site        = $split[0];
	$environment = $split[1];
}

if( strpos( $site, "@" ) !== false ) {
	$split       = explode( "@", $site );
	$site        = $split[0];
	$provider    = $split[1];
}

if( strpos( $environment, "@" ) !== false ) {
	$split       = explode( "@", $environment );
	$environment = $split[0];
	$provider    = $split[1];
}

// Assign default format to JSON
if ( empty( $format ) ) {
	$format = "json";
}
foreach( [ "once" ] as $run ) {
	if ( is_numeric( $site ) ) {
		$lookup = ( new CaptainCore\Sites )->where( [ "site_id" => $site, "status" => "active" ] );
		continue;
	}
	if ( ! empty( $provider ) ) {
		$lookup = ( new CaptainCore\Sites )->where( [ "site" => $site, "provider" => $provider, "status" => "active" ] );
		continue;
	}
	$lookup = ( new CaptainCore\Sites )->where( [ "site" => $site, "status" => "active" ] );
}

// Error if site not found
if ( count( $lookup ) == 0 ) {
	return "";
}

// Fetch site
$site               = ( new CaptainCore\Site( $lookup[0]->site_id ) )->get();
$site               = (object) $site;
$site->environments = ( new CaptainCore\Site( $lookup[0]->site_id ) )->environments();

// Set environment if not defined
if ( empty( $environment ) ) {
	$environment = "Production";
}

$environment_key         = array_search( ucfirst($environment), array_column( $site->environments, 'environment' ) );
$address                 = $site->environments[$environment_key]->address;
$username                = $site->environments[$environment_key]->username;
$password                = $site->environments[$environment_key]->password;
$protocol                = $site->environments[$environment_key]->protocol;
$port                    = $site->environments[$environment_key]->port;
$home_directory          = $site->environments[$environment_key]->home_directory;
$database_username       = $site->environments[$environment_key]->database_username;
$database_password       = $site->environments[$environment_key]->database_password;
$capture_pages           = $site->environments[$environment_key]->capture_pages;
$details                 = empty( $site->environments[$environment_key]->details ) ? (object) [] : $site->environments[$environment_key]->details;
$fathom                  = $site->environments[$environment_key]->fathom;
$offload_enabled         = $site->environments[$environment_key]->offload_enabled;
$offload_provider        = $site->environments[$environment_key]->offload_provider;
$offload_access_key      = $site->environments[$environment_key]->offload_access_key;
$offload_secret_key      = $site->environments[$environment_key]->offload_secret_key;
$offload_bucket          = $site->environments[$environment_key]->offload_bucket;
$offload_path            = $site->environments[$environment_key]->offload_path;
$home_url                = $site->environments[$environment_key]->home_url;
$monitor_enabled         = $site->environments[$environment_key]->monitor_enabled;
$updates_enabled         = $site->environments[$environment_key]->updates_enabled;
$updates_exclude_themes  = $site->environments[$environment_key]->updates_exclude_themes;
$updates_exclude_plugins = $site->environments[$environment_key]->updates_exclude_plugins;
$wp_content              = "wp-content";
$environment_vars        = "";

if ( is_array( $site->environment_vars ) ) { 
	foreach ( $site->environment_vars as $item ) {
		$environment_vars = "{$environment_vars} {$item->key}='{$item->value}'";
		if ( $item->key == "STACKED_ID" || $item->key == "STACKED_SITE_ID" ) {
			$wp_content = "content/{$item->value}";
		}
	}
	$environment_vars = "export $environment_vars";
}

$array = [
	"site_id"                 => $site->site_id,
	"site"                    => $site->site,
	"status"                  => empty( $site->status ) ? "" : $site->status,
	"provider"                => $site->provider,
	"key"                     => $site->key,
	"environment_vars"        => empty( $environment_vars ) ? "" : $environment_vars,
	"domain"                  => $site->name,
	"home_url"                => $home_url,
	"defaults"                => empty( $site->account["defaults"] ) ? "[]" : json_encode( $site->account["defaults"] ),
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
	'monitor_enabled'         => empty( $monitor_enabled ) ? 0 : $monitor_enabled,
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
	$capture_pages = implode(",", array_column( $capture_pages, "page" ) );
}

if ( empty( $details->auth ) ) {
	$details->auth = "";
} else {
	$details->auth = base64_encode( $details->auth->username . ":" .  $details->auth->password );
}

if ( $format == 'bash' && is_array( $fathom ) ) {
	if ( empty( $fathom[0]->domain ) || empty( $fathom[0]->code ) ) {
		$fathom = "";
	} else {
		$fathom = json_encode( $fathom );
	}
}

$default_users = empty( $site->account["defaults"]->users ) ? "[]" : json_encode ( $site->account["defaults"]->users );

if ( is_array( $updates_exclude_themes ) ) {
	$updates_exclude_themes = implode( ",", $updates_exclude_themes );
}
if ( is_array( $updates_exclude_plugins ) ) {
	$updates_exclude_plugins = implode( ",", $updates_exclude_plugins );
}

if ( ! empty( $args["field"] ) ) {
	echo $array[$args["field"]];
	return true;
}

if ( $format == 'json' ) {
	echo json_encode( $array, JSON_PRETTY_PRINT );
	return;
}

if ( empty( $environment_vars ) ) { $environment_vars = ""; }
if ( empty( $site->status ) ) { $site->status = ""; }

$backup = ( new CaptainCore\Site( $site->site_id ) )->fetch()->backup_settings;

$bash = "site_id={$site->site_id}
domain={$site->name}
key={$site->key}
fathom=$fathom
capture_pages=$capture_pages
site={$site->site}
auth={$details->auth}
environment_vars={$environment_vars}
wp_content={$wp_content}
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
backup_active={$backup->active}
backup_interval={$backup->interval}
backup_mode={$backup->mode}";

if ( $format == 'bash' ) {
	echo $bash;
}