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

// Determines environment
if ( strpos($site, '-staging') !== false ) {
    $site        = str_replace( "-staging", "", $site );
    $environment = "staging";
} else {
    $site        = str_replace( "-production", "", $site );
    $environment = "production";
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
$json = "{$_SERVER['HOME']}/.captaincore-cli/config.json";

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

$fathom = json_decode( $environment->fathom );

if ( is_array( $fathom ) ) {
    if ( is_string( $fathom[0]->code ) ) {
        $fathom = $fathom[0]->code;
    } else {
        $fathom = $fathom[0];
    }
}

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
        set_transient( "captaincore_fathom_auth_{$captain_id}", $auth, HOUR_IN_SECONDS );

    }

    // Load sites from transient
    $sites = get_transient( "captaincore_fathom_sites_{$captain_id}" );

    // If empty then update transient with large remote call
    if ( empty( $sites ) ) {

        // Fetch Sites
        $response = wp_remote_get( "$fathom_instance/api/sites", [ 'cookies' => $auth['cookies'] ] );
        $sites    = json_decode( $response['body'] )->Data;

        // Save the API response so we don't have to call again until tomorrow.
        set_transient( "captaincore_fathom_sites_{$captain_id}", $sites, HOUR_IN_SECONDS );

    }

    foreach( $sites as $s ) {
        if ( $s->trackingId == $fathom ) {
            // Fetch 12 months of stats from today
            $before   = strtotime( "now" );
            $after    = strtotime( "-12 months" );
            $response = wp_remote_get( "$fathom_instance/api/sites/{$s->id}/stats/site?before=$before&after=$after", [ 'cookies' => $auth['cookies'] ] );
            $stats    = json_decode( $response['body'] )->Data;
        }
    }

    if ( $stats ) {
        $total_pageviews = 0;
        foreach( $stats as $stat ){
            if(isset($stat->Pageviews))
               $total_pageviews += $stat->Pageviews;
          }
        // Return yearly average based on current usage.
        echo $total_pageviews;
    } else {
        // Fathom not found
        echo "0";
    }
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