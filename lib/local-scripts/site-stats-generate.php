<?php

$captain_id             = getenv('CAPTAIN_ID');
$skip_already_generated = getenv('SKIP_ALREADY_GENERATED');
$site                   = $args[0];

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

// Determines environment
if ( strpos($site, '-staging') !== false ) {
    $site        = str_replace( "-staging", "", $site );
    $environment_name = "staging";
} else {
    $site        = str_replace( "-production", "", $site );
    $environment_name = "production";
}

$lookup = ( new CaptainCore\Sites )->where( [ "site" => $site ] );

// Error if site not found
if ( count( $lookup ) == 0 ) {
	echo "Error: Site '$site' not found.";
	return;
}

// Fetch site
$site           = ( new CaptainCore\Sites )->get( $lookup[0]->site_id );
$environment_id = ( new CaptainCore\Site( $site->site_id ) )->fetch_environment_id( $environment_name );
$environment    = ( new CaptainCore\Environments )->get( $environment_id );

if ( ! empty( $skip_already_generated ) ) {
    $details         = ( isset( $environment->details ) ? json_decode( $environment->details ) : (object) [] );
    if ( ! empty( $details->fathom ) && ! empty( $details->fathom[0]->code ) ) {
        echo "Skipping {$site->site}-{$environment_name} as tracking ID already exists\n";
        return;
    }
}

if ( empty( $environment->home_url ) ) {
    echo "Error: WordPress not found for {$site->site}-{$environment_name}\n";
	return;
}

// Fetch site name
if ( $environment_name == "production" ) {
    $site_name = $site->name;
}
if ( $environment_name == "staging" ) {
    $site_name = shell_exec( "captaincore ssh {$site->site}-{$environment_name} --command=\"wp option get home --skip-plugins --skip-themes\" --captain-id=$captain_id" );
    $site_name = str_replace( "http://", "", $site_name );
    $site_name = trim ( str_replace( "https://", "", $site_name ) );
}

// If site name missing then do not proceed
if ( $site_name == "" || strpos($site_name, ':') !== false ) {
    return;
}

$json        = $_SERVER['HOME'] . "/.captaincore/config.json";
$config_data = json_decode ( file_get_contents( $json ) );
$system      = $config_data[0]->system;

foreach($config_data as $config) {
	if ( isset( $config->captain_id ) and $config->captain_id == $captain_id ) {
		$configuration = $config;
		break;
	}
}

$time_now        = date("Y-m-d H:i:s");
$analytics       = new BeyondCode\FathomAnalytics\FathomAnalytics( $system->fathom_username, $system->fathom_password );
$response        = $analytics->newSite( $site_name );
$details         = ( isset( $environment->details ) ? json_decode( $environment->details ) : (object) [] );
if ( empty( $response->tracking_id ) ) {
    echo "Error: Could not fetch tracking ID from Fathom for {$site->site}-{$environment_name}\n";
	return;
}
$details->fathom = [ [ "domain" => $site_name, "code" => $response->tracking_id ] ];
( new CaptainCore\Environments )->update( [ 
    "details"         => json_encode( $details ),
    "updated_at"      => $time_now,
], [ "environment_id" => $environment_id ] );

// Prepare request to API
$request = [
    'method'  => 'POST',
    'headers' => [ 'Content-Type' => 'application/json' ],
    'body'    => json_encode( [ 
        "command" => "update-fathom",
        "site_id" => $site->site_id,
        "token"   => $configuration->keys->token,
        "data"    => [ "fathom" => $details->fathom, "environment_id" => $environment_id ],
    ] ),
];

if ( $system->captaincore_dev ) {
    $request['sslverify'] = false;
}

// Post to CaptainCore API
$response = wp_remote_post( $configuration->vars->captaincore_api, $request );
echo $response['body'] . "\n";

// Deploy tracker
echo shell_exec( "captaincore stats-deploy {$site->site}-${environment_name} --captain-id=$captain_id" );
