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

// Determines environment
if ( strpos($site, '-staging') !== false ) {
    $site        = str_replace( "-staging", "", $site );
    $environment = "staging";
} else {
    $site        = str_replace( "-production", "", $site );
    $environment = "production";
}

if ( empty( $site ) ) {
    $COLOR_RED    = "\033[31m";
    $COLOR_NORMAL = "\033[39m";

    echo "${COLOR_RED}Error:${COLOR_NORMAL} Please specify a <site>.\n";
    return;
}

$lookup = ( new CaptainCore\Sites )->where( [ "site" => $site ] );

// Error if site not found
if ( count( $lookup ) == 0 ) {
	echo "Error: Site '$site' not found.";
	return;
}

// Fetch site
$site           = ( new CaptainCore\Sites )->get( $lookup[0]->site_id );
$environment_id = ( new CaptainCore\Site( $site->site_id ) )->fetch_environment_id( $environment );
$environment    = ( new CaptainCore\Environments )->get( $environment_id );

// Loads CLI configs
$json = "{$_SERVER['HOME']}/.captaincore/config.json";

if ( ! file_exists( $json ) ) {
	echo "Error: Configuration file not found.";
	return;
}

$config_data = json_decode ( file_get_contents( $json ) );
$system      = $config_data[0]->system;
foreach($config_data as $config) {
	if ( isset( $config->captain_id ) and $config->captain_id == $captain_id ) {
		$configuration = $config;
		break;
	}
}

$fathom = json_decode( $environment->fathom );

if ( is_array( $fathom ) ) {
    if ( is_string( $fathom[0]->code ) ) {
        $fathom = $fathom[0]->code;
    } else {
        $fathom = $fathom[0];
    }
}

$details          = ( isset( $environment->details ) ? json_decode( $environment->details ) : (object) [] );
$fathom_analytics = ( ! empty( $details->fathom ) ? $details->fathom : [] );
$fathom_ids       = array_column( $fathom_analytics, "code" );

// hunt by site name
if ( empty( $fathom_analytics ) || count( $fathom_ids ) != 1 ) {
    $hunt          = null;
    $site_name     = $environment->home_url;
    $site_name     = str_replace( "http://www.", "", $site_name );
    $site_name     = str_replace( "https://www.", "", $site_name );
    $site_name     = str_replace( "http://", "", $site_name );
    $site_name     = str_replace( "https://", "", $site_name );
    defined('FATHOM_API_KEY') or define( 'FATHOM_API_KEY', $system->fathom_api_key );
    $fathom_sites = ( new CaptainCore\Sites )->fathom_sites();
    foreach ( $fathom_sites as $fathom_site ) {
        if ( $fathom_site->name == $site_name ) {
            $hunt = $fathom_site;
            break;
        }
    }
    if ( ! empty( $hunt ) ) {
        $fathom_id  = $fathom_site->id;
    }
}

if ( empty( $fathom_id ) ) {
    $fathom_id = $fathom_ids[0];
}
$after    = date( 'Y-m-d H:i:s' );
$date     = strtotime("$after -1 year" );
$before   = date('Y-m-d H:i:s', $date);
$grouping = "month";
$url      = "https://api.usefathom.com/v1/aggregations?entity=pageview&entity_id=$fathom_id&aggregates=visits,pageviews,avg_duration,bounce_rate&date_from=$before&date_to=$after&date_grouping=$grouping&sort_by=timestamp:asc";
$response = wp_remote_get( $url, [ 
    "headers" => [ "Authorization" => "Bearer " .  $system->fathom_api_key ],
] );

$stats           = json_decode( $response['body'] );
$total_pageviews = array_sum( array_column( $stats, "pageviews" ) );

// If Fathom found then fetch stats
if ( $fathom != "" ) {
    
    $fathom_instance = "https://{$configuration->vars->captaincore_tracker_api}";
    $login_details   = [
        'email'    => $configuration->vars->captaincore_tracker_user,
        'password' => $configuration->vars->captaincore_tracker_pass
    ];

    // Load sites from transient
    $auth = get_transient( "captaincore_fathom_auth_{$captain_id}" );

    // If empty then update transient with large remote call
    if ( empty( $auth ) ) {

        // Authenticate to Fathom instance

        $auth = wp_remote_post( "$fathom_instance/api/session", [ 
            'method'  => 'POST',
            'headers' => [ 'Content-Type' => 'application/json; charset=utf-8' ],
            'body'    => json_encode( $login_details )
         ] );

        // Save the API response so we don't have to call again until tomorrow.
        set_transient( "captaincore_fathom_auth_{$captain_id}", $auth, 20 * MINUTE_IN_SECONDS );

    }

    // Load sites from transient
    $sites = get_transient( "captaincore_fathom_sites_{$captain_id}" );

    // If empty then update transient with large remote call
    if ( empty( $sites ) ) {

        // Fetch Sites
        $response = wp_remote_get( "$fathom_instance/api/sites", [ 'cookies' => $auth['cookies'] ] );
        $sites    = json_decode( $response['body'] )->Data;

        // Save the API response so we don't have to call again until tomorrow.
        set_transient( "captaincore_fathom_sites_{$captain_id}", $sites, 20 * MINUTE_IN_SECONDS );

    }

    foreach( $sites as $s ) {
        if ( $s->trackingId == $fathom ) {
            // Fetch 12 months of stats from today
            $before   = ( empty( $configuration->vars->captaincore_tracker_cutoff ) ? strtotime( "now" ) : strtotime( $configuration->vars->captaincore_tracker_cutoff ) );
            $after    = strtotime( "-12 months" );
            $response = wp_remote_get( "$fathom_instance/api/sites/{$s->id}/stats/site?before=$before&after=$after", [ 'cookies' => $auth['cookies'] ] );
            $stats    = json_decode( $response['body'] )->Data;
        }
    }

    if ( $stats ) {
        foreach( $stats as $stat ){
            if(isset($stat->Pageviews))
               $total_pageviews += $stat->Pageviews;
          }
    }

}

if ( ! empty( $total_pageviews ) ) {
    echo $total_pageviews;
    die();
}

// Attempt to fetch from WordPress.com
if ( $fathom == "" ) {

    // Connects to WordPress.com and pulls blog ids
    $keys = json_decode( shell_exec( "captaincore configs fetch keys --captain_id=$captain_id" ) );
    $access_key = $keys->access_key;

    if ( $site_details->domain ) {

        // Define vars
        $count  = 0;
        $total  = 0;
        $months = '';

        // Pull stats from WordPress API
        $curl = curl_init( "https://public-api.wordpress.com/rest/v1/sites/{$site_details->domain}/stats/visits?unit=month&quantity=12" );
        curl_setopt( $curl, CURLOPT_HTTPHEADER, [ 'Authorization: Bearer ' . $access_key ] );
        curl_setopt( $curl, CURLOPT_RETURNTRANSFER, 1 );
        $response = curl_exec( $curl );
        $stats    = json_decode( $response, true );

        if ( isset( $stats['error'] ) and $stats['error'] == 'unknown_blog' ) {
            // Attempt to load www version
            // Pull stats from WordPress API
            $curl = curl_init( "https://public-api.wordpress.com/rest/v1/sites/www.{$site_details->domain}/stats/visits?unit=month&quantity=12" );
            curl_setopt( $curl, CURLOPT_HTTPHEADER, [ 'Authorization: Bearer ' . $access_key ] );
            curl_setopt( $curl, CURLOPT_RETURNTRANSFER, 1 );
            $response = curl_exec( $curl );
            $stats    = json_decode( $response, true );

        }

        if ( isset( $stats['data'] ) ) {

            // Preps views for last 12 months for html output while calculating usage.
            foreach ( $stats['data'] as $stat ) {
                if ( $stat[0] ) {
                    $total   = $total + 1;
                    $months .= $stat[0] . ' - ' . $stat[1] . '<br />';
                }
                $count = $count + $stat[1];
            }

            if ( $total == 0 ) {
                $monthly_average = 0;
            } else {
                $monthly_average = round( $count / $total );
            }

            $yearly_estimate = $monthly_average * 12;
            echo $yearly_estimate;

        } else {
            // Error so return 0. For debug info see print_r($stats);
            echo '0';
        }
    }
}