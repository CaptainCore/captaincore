<?php

$COLOR_RED    = "\033[31m";
$COLOR_NORMAL = "\033[39m";
$site         = $args[0];
$captain_id   = getenv('CAPTAIN_ID');

// Replaces dashes in keys with underscores
foreach( $args as $index => $arg ) {
	$split = strpos( $arg, "=" );
	if ( $split ) {
		$key   = str_replace('-', '_', substr( $arg , 0, $split ) );
		$value = substr( $arg , $split, strlen( $arg ) );

		// Removes unnecessary bash quotes
		$value = trim( $value,'"' ); 				// Remove last quote 
		$value = str_replace( '="', '=', $value );  // Remove quote right after equals

		$args[$index] = $key.$value;
	} else {
		$args[$index] = str_replace('-', '_', $arg);
	}
}

parse_str( implode( '&', $args ) );

// Loads CLI configs
$json = "{$_SERVER['HOME']}/.captaincore/config.json";

if ( ! file_exists( $json ) ) {
	echo "Error: Configuration file not found.";
	return;
}

$config_data = json_decode ( file_get_contents( $json ) );
foreach($config_data as $config) {
	if ( isset( $config->captain_id ) and $config->captain_id == $captain_id ) {
		$configuration = $config;
		break;
	}
}
$system      = $config_data[0]->system;
$path        = $system->path;

if ( strpos($site, '@') !== false ) {
    $parts    = explode( "@", $site );
    $site     = $parts[0];
    $provider = $parts[1];
}

// Determines environment
if ( strpos($site, '-staging') !== false ) {
    $site = str_replace( "-staging", "", $site );
    $env  = "staging";
} else {
    $site = str_replace( "-production", "", $site );
    $env  = "production";
}

if ( empty( $site ) ) {
    echo "${COLOR_RED}Error:${COLOR_NORMAL} Please specify a <site>.\n";
    return;
}

$lookup = ( new CaptainCore\Sites )->where( [ "site" => $site ] );

// Error if site not found
if ( count( $lookup ) == 0 ) {
	echo "${COLOR_RED}Error:${COLOR_NORMAL} Site '$site' not found.\n";
	return;
}

// Fetch site
$site           = ( new CaptainCore\Sites )->get( $lookup[0]->site_id );
$environment_id = ( new CaptainCore\Site( $site->site_id ) )->fetch_environment_id( $env );
$environment    = ( new CaptainCore\Environments )->get( $environment_id );
$details        = ( isset( $environment->details ) ? json_decode( $environment->details ) : (object) [] );

if ( empty( $environment ) ) {
    echo "${COLOR_RED}Error:${COLOR_NORMAL} Environment $environment->environment not found for '$site->name'.\n";
	return;
}

if ( empty( $environment->address ) ) {
    echo "${COLOR_RED}Error:${COLOR_NORMAL} Environment $environment->environment not found for '$site->name'.\n";
    return;
}

if ( $environment->protocol != "sftp" ) {
    echo "${COLOR_RED}Error:${COLOR_NORMAL} SSH not supported (Protocol is $environment->protocol).";
    return;
}

if ( $environment->provider != "kinsta" && $environment->address == "" ) {
    echo "${COLOR_RED}Error:${COLOR_NORMAL} Missing Kinsta site.";
    return;
}

if ( $environment->provider == "wpengine" && $environment == "staging" ) {
    echo "${COLOR_RED}Error:${COLOR_NORMAL} Missing WP Engine site.";
    return;
}

if ( $environment->provider == "wpengine" && $environment == "staging" ) {
    echo "${COLOR_RED}Error:${COLOR_NORMAL} Missing WP Engine site.";
    return;
}

$site_data        = json_decode( $site->details );
$wp_content = "wp-content";
if ( is_array( $site_data->environment_vars ) ) { 
	foreach ( $site_data->environment_vars as $item ) { 
		if ( $item->key == "STACKED_ID" || $item->key == "STACKED_SITE_ID" ) {
			$wp_content = "content/{$item->value}";
		}
	}
}

$fathom_analytics = ( ! empty( $details->fathom ) ? json_encode( $details->fathom ) : [] );
$fathom_arguments = "id=$fathom_analytics\nbranding_author={$configuration->vars->captaincore_branding_author}\nbranding_author_uri={$configuration->vars->captaincore_branding_author_uri}\nbranding_slug={$configuration->vars->captaincore_branding_slug}";
$fathom_arguments = base64_encode( $fathom_arguments );

echo shell_exec( "captaincore ssh {$site->site}-{$env} --script=deploy-fathom -- --wp_content=$wp_content --fathom_arguments=$fathom_arguments" );