<?php

$captain_id = getenv('CAPTAIN_ID');
$debug      = getenv('CAPTAINCORE_DEBUG');

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
parse_str( implode( '&', $args ), $arguments );
$arguments               = (object) $arguments;
$site                    = $arguments->site;
$environment             = $arguments->environment;
$updates_enabled         = $arguments->updates_enabled;
$updates_exclude_themes  = $arguments->updates_exclude_themes;
$updates_exclude_plugins = $arguments->updates_exclude_plugins;

$lookup = ( new CaptainCore\Sites )->where( [ "site" => $site ] );

// Error if site not found
if ( count( $lookup ) == 0 ) {
	echo "Error: Site '$site' not found.";
	return;
}

$site           = ( new CaptainCore\Sites )->get( $lookup[0]->site_id );
$environment_id = ( new CaptainCore\Site( $site->site_id ) )->fetch_environment_id( $environment );

foreach( [ "once" ] as $run ) {
    if ( $updates_exclude_themes != "" && $updates_exclude_plugins != "" ) {
        $command = "captaincore ssh {$site->site}-{$environment} --script=update --captain-id=$captain_id -- --exclude_plugins=$updates_exclude_plugins --exclude_themes=$updates_exclude_themes --all --format=json --provider={$site->provider}";
        continue;
    }
    if ( $updates_exclude_themes != "" ) {
        $command = "captaincore ssh {$site->site}-{$environment} --script=update --captain-id=$captain_id -- --exclude_themes=$updates_exclude_themes --all --format=json --provider={$site->provider}";
        continue;
    }
    if ( $updates_exclude_plugins != "" ) {
        $command = "captaincore ssh {$site->site}-{$environment} --script=update --captain-id=$captain_id -- --exclude_plugins=$updates_exclude_plugins --all --format=json --provider={$site->provider}";
        continue;
    }
    $command = "captaincore ssh {$site->site}-{$environment} --script=update --captain-id=$captain_id -- --all --format=json --provider={$site->provider}";
}

if ( $debug == "true" ) {
    echo "$command\n";
    exit;
}

$response = shell_exec( $command );