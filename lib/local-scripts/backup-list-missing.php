<?php

$captain_id = getenv('CAPTAIN_ID');
$site       = $args[0];

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
if ( $format == "" ) {
	$format = "json";
}
foreach( [ "once" ] as $run ) {
	if ( $provider ) {
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

// Set environment if not defined
if ( $environment == "" ) {
	$environment = "production";
}

// Fetch site
$site = ( new CaptainCore\Site( $lookup[0]->site_id ) )->get();

// Loads CLI configs
$json = "{$_SERVER['HOME']}/.captaincore/config.json";

if ( ! file_exists( $json ) ) {
	echo "Error: Configuration file not found.";
	return;
}

$config_data = json_decode ( file_get_contents( $json ) );
$system      = $config_data[0]->system;

if ( $system->captaincore_fleet == "true" ) {
    $system->path            = "{$system->path}/${captain_id}";
    $system->rclone_backup   = "{$system->rclone_backup}/{$captain_id}";
}

if ( is_file ( "$system->path/{$site->site}_{$site->site_id}/{$environment}/backups/list.json" ) ) {
	$snapshots    = json_decode ( file_get_contents( "$system->path/{$site->site}_{$site->site_id}/{$environment}/backups/list.json" ) );
	$snapshot_ids = array_column( $snapshots, "id" );
	foreach ( $snapshot_ids as $snapshot_id ) {
		if ( ! is_file ( "$system->path/{$site->site}_{$site->site_id}/{$environment}/backups/snapshot-$snapshot_id.json" ) ) {
			echo "Generating missing {$site->site}_{$site->site_id}/{$environment}/backups/snapshot-$snapshot_id.json\n";
			shell_exec( "captaincore backup get-generate {$site->site}-{$environment} $snapshot_id --captain-id=$captain_id" );
		}
	}
}