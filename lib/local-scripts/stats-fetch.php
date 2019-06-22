<?php

// Converts --arguments into $arguments
parse_str( implode( '&', $args ) );

$site = $args[0];
$site_details = json_decode( shell_exec( "captaincore site get $site --captain_id=$captain_id" ) );
$configuration = json_decode( shell_exec( "captaincore configs fetch vars --captain_id=$captain_id" ) );

// Determines environment
if ( strpos($site, '-staging') !== false ) {
    $environment = "staging";
    $site_name = $site_details->home_url;
    $site_name = str_replace( "http://", "", $site_name );
    $site_name = trim ( str_replace( "https://", "", $site_name ) );
} else {
    $environment = "production";
    $site_name = $site_details->domain;
}

$fathom = json_decode( $site_details->fathom );

// If Fathom found then fetch stats
if ( count($fathom) > 0 ) {
    
    $fathom_instance = "https://{$configuration->captaincore_tracker}";

    $login_details = array(
            'email'    => $configuration->captaincore_tracker_user, 
            'password' => $configuration->captaincore_tracker_pass
    );

    // Load sites from transient
    $auth = get_transient( "captaincore_fathom_auth_{$captain_id}" );

    // If empty then update transient with large remote call
    if ( empty( $auth ) ) {

        // Authenticate to Fathom instance
        $auth = wp_remote_post( "$fathom_instance/api/session", array( 
            'method'  => 'POST',
            'headers' => array( 'Content-Type' => 'application/json; charset=utf-8' ),
            'body'    => json_encode( $login_details )
        ) );

        // Save the API response so we don't have to call again until tomorrow.
        set_transient( "captaincore_fathom_auth_{$captain_id}", $auth, HOUR_IN_SECONDS );

    }

    // Load sites from transient
    $sites = get_transient( "captaincore_fathom_sites_{$captain_id}" );

    // If empty then update transient with large remote call
    if ( empty( $sites ) ) {

        // Fetch Sites
        $response = wp_remote_get( "$fathom_instance/api/sites", array( 
            'cookies' => $auth['cookies']
        ) );
        $sites = json_decode( $response['body'] )->Data;

        // Save the API response so we don't have to call again until tomorrow.
        set_transient( 'captaincore_fathom_sites', $sites, HOUR_IN_SECONDS );

    }

    foreach( $sites as $s ) {
        if ( $s->name == $site_name ) {
            // Fetch 12 months of stats from today
            $before = strtotime( "now" );
            $after  = strtotime( "-12 months" );
            $response = wp_remote_get( "$fathom_instance/api/sites/{$s->id}/stats/site?before=$before&after=$after", array(
                'cookies' => $auth['cookies']
            ) );
            $stats = json_decode( $response['body'] )->Data;
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
if ( count($fathom) == 0 ) {


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
        curl_setopt( $curl, CURLOPT_HTTPHEADER, array( 'Authorization: Bearer ' . $access_key ) );
        curl_setopt( $curl, CURLOPT_RETURNTRANSFER, 1 );
        $response = curl_exec( $curl );
        $stats    = json_decode( $response, true );

        if ( isset( $stats['error'] ) and $stats['error'] == 'unknown_blog' ) {
            // Attempt to load www version
            // Pull stats from WordPress API
            $curl = curl_init( "https://public-api.wordpress.com/rest/v1/sites/www.{$site_details->domain}/stats/visits?unit=month&quantity=12" );
            curl_setopt( $curl, CURLOPT_HTTPHEADER, array( 'Authorization: Bearer ' . $access_key ) );
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