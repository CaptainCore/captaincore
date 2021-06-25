<?php

$captain_id = getenv('CAPTAIN_ID');

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

$lookup = ( new CaptainCore\Sites )->where( [ "site" => $site ] );

// Error if site not found
if ( count( $lookup ) == 0 ) {
	echo "Error: Site '$site' not found.";
	return;
}

$site           = ( new CaptainCore\Sites )->get( $lookup[0]->site_id );
$environment_id = ( new CaptainCore\Site( $site->site_id ) )->fetch_environment_id( $environment );
$environment    = ( new CaptainCore\Environments )->get( $environment_id );

$json        = "{$_SERVER['HOME']}/.captaincore/config.json";
$config_data = json_decode ( file_get_contents( $json ) );
$system      = $config_data[0]->system;

foreach($config_data as $config) {
	if ( isset( $config->captain_id ) and $config->captain_id == $captain_id ) {
		$configuration = $config;
		break;
	}
}

foreach( [ "once" ] as $run ) {
    $results = shell_exec( "lighthouse {$environment->home_url} --only-audits=errors-in-console --chrome-flags=\"--headless\" --output=json --quiet" );
   
    // Check if JSON valid
    if ( json_last_error() !== JSON_ERROR_NONE ) {
        echo "Check not valid format";
        continue;
    }

    $results = json_decode( $results );
    $details = json_decode( $environment->details );

    if ( isset( $results->audits->{'errors-in-console'}->details->items ) && ! empty( $results->audits->{'errors-in-console'}->details->items ) ) {

        $details->console_errors = $results->audits->{'errors-in-console'}->details->items;
        $environment_update = [
            "environment_id" => $environment_id,
            "details"        => json_encode( $details ),
            "updated_at"     => date("Y-m-d H:i:s"),
        ];
        
        echo "Detected " . count( $details->console_errors ). " errors on $home_url\n";
        echo json_encode( $details->console_errors, JSON_PRETTY_PRINT );
        
        ( new CaptainCore\Environments )->update( $environment_update, [ "environment_id" => $environment_id ] );

        // Prepare request to API
        $request = [
            'method'  => 'POST',
            'headers' => [ 'Content-Type' => 'application/json' ],
            'body'    => json_encode( [ 
                "command" => "update-environment",
                "site_id" => $site->site_id,
                "token"   => $configuration->keys->token,
                "data"    => $environment_update,
            ] ),
        ];

        if ( $system->captaincore_dev ) {
            $request['sslverify'] = false;
        }

        $response = wp_remote_post( $configuration->vars->captaincore_api, $request );

        $details  = json_decode( $site->details );
        $details->console_errors = $results->audits->{'errors-in-console'}->details->items;
        $site_update = [
            "site_id" => $site->site_id,
            "details" => json_encode( $details )
        ];
        ( new CaptainCore\Sites )->update( $site_update, [ "site_id" => $site->site_id ] );

        // Prepare request to API
        $request = [
            'method'  => 'POST',
            'headers' => [ 'Content-Type' => 'application/json' ],
            'body'    => json_encode( [ 
                "command" => "update-site",
                "site_id" => $site->site_id,
                "token"   => $configuration->keys->token,
                "data"    => $site_update,
            ] ),
        ];

        if ( $system->captaincore_dev ) {
            $request['sslverify'] = false;
        }

        $response = wp_remote_post( $configuration->vars->captaincore_api, $request );
        echo $response['body'];
        continue;
    }

    // No errors, empty existing if needed
    if ( ! empty( $details->console_errors ) ) {

        // Remove errors
        unset( $details->console_errors );

        // Prepare request to API
        $request = [
            'method'  => 'POST',
            'headers' => [ 'Content-Type' => 'application/json' ],
            'body'    => json_encode( [ 
                "command" => "update-environment",
                "site_id" => $site->site_id,
                "token"   => $configuration->keys->token,
                "data"    => [ "environment_id" => $environment_id, "details" => json_encode( $details ), "updated_at" => date("Y-m-d H:i:s") ],
            ] ),
        ];

        if ( $system->captaincore_dev ) {
            $request['sslverify'] = false;
        }

        // Post to CaptainCore API
        $response = wp_remote_post( $configuration->vars->captaincore_api, $request );
        echo $response['body'];
    }

}

