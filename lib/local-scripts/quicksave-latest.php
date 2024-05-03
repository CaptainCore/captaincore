<?php

$captain_id = getenv('CAPTAIN_ID');

// Converts --arguments into $arguments
parse_str( implode( '&', $args ), $arguments );
$arguments = (object) $arguments;

$site = $args[0];

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

if( isset( $environment ) && strpos( $environment, "@" ) !== false ) {
	$split       = explode( "@", $environment );
	$environment = $split[0];
	$provider    = $split[1];
}

// Assign default format to JSON
if ( empty( $format ) ) {
	$format = "json";
}
foreach( [ "once" ] as $run ) {
	if ( ! empty( $provider ) ) {
		$lookup = ( new CaptainCore\Sites )->where( [ "site" => $site, "provider" => $provider, "status" => "active" ] );
		continue;
	}
	if ( ctype_digit( $site ) ) {
		$lookup = ( new CaptainCore\Sites )->where( [ "site_id" => $site, "status" => "active" ] );
		continue;
	}
	$lookup = ( new CaptainCore\Sites )->where( [ "site" => $site, "status" => "active" ] );
}

// Error if site not found
if ( count( $lookup ) == 0 ) {
	return "";
}

// Fetch site
$site = ( new CaptainCore\Site( $lookup[0]->site_id ) )->get();

// Set environment if not defined
if ( empty( $environment ) ) {
	$environment = "production";
}

// Loads CLI configs
$json = "{$_SERVER['HOME']}/.captaincore/config.json";

if ( ! file_exists( $json ) ) {
	echo "Error: Configuration file not found.";
	return;
}

$config_data = json_decode ( file_get_contents( $json ) );
$system      = $config_data[0]->system;

foreach($config_data as $config) {
	if ( isset ( $config->captain_id ) and $config->captain_id == $captain_id ) {
		$configuration = $config;
		break;
	}
}

if ( $system->captaincore_fleet == "true" ) {
    $system->path            = "{$system->path}/${captain_id}";
}

$quicksave_list = "$system->path/{$site->site}_{$site->site_id}/{$environment}/quicksaves/list.json";

if ( ! is_file( $quicksave_list ) ) {
    return;
}

$list   = json_decode( file_get_contents( $quicksave_list ) );
$recent = $list[0];

if ( ! empty( $arguments->field ) ) {
    echo $recent->{$arguments->field};
    return;
}


echo json_encode( $recent, JSON_PRETTY_PRINT );
