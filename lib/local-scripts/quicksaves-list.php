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
parse_str( implode( '&', $args ), $args );
$site        = $args["site"];
$site_id     = $args["site_id"];
$environment = $args["environment"];

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

$command    = "cd $system->path/{$site}_{$site_id}/{$environment}/quicksave/; git log --pretty='format:%H %ct'";
$response   = shell_exec( $command );
$quicksaves = explode( "\n", trim( $response ) );
foreach ( $quicksaves as $key => $quicksave ) {
    $split            = explode( " ", $quicksave );
    if ( empty(  $split[0] ) || empty(  $split[1] ) || $split[0] == "-n" ) {
        unset( $quicksaves[$key]);
        continue;
    }
    $quicksave_item = (object) [ 
        "hash"       => $split[0],
        "created_at" => $split[1]
    ];
    if ( is_file( "$system->path/{$site}_{$site_id}/{$environment}/quicksaves/commit-{$split[0]}.json" ) ) {
        $quicksave_data = json_decode( file_get_contents( "$system->path/{$site}_{$site_id}/{$environment}/quicksaves/commit-{$split[0]}.json" ) );
        if ( $quicksave_data->core ) {
            $quicksave_item->core = $quicksave_data->core;
        }
        if ( $quicksave_data->theme_count ) {
            $quicksave_item->theme_count = $quicksave_data->theme_count;
        }
        if ( $quicksave_data->plugin_count ) {
            $quicksave_item->plugin_count = $quicksave_data->plugin_count;
        }
        if ( $quicksave_data->core ) {
            $quicksave_item->core = $quicksave_data->core;
        }
        if ( $quicksave_data->core_previous ) {
            $quicksave_item->core_previous = $quicksave_data->core_previous;
        }
    }
    $quicksaves[$key] = $quicksave_item;
}

echo json_encode( $quicksaves, JSON_PRETTY_PRINT );

if ( empty ( count( $quicksaves ) ) ) {
    $error = [
        "response" => $response,
        "command"  => $command,
    ];
    $error_file = "{$_SERVER['HOME']}/.captaincore/data/quicksave-error-{$site}_{$site_id}_{$environment}.log";
    file_put_contents( $error_file, json_encode( $error, JSON_PRETTY_PRINT ) );
}

$environment_id = ( new CaptainCore\Site( $site_id ) )->fetch_environment_id( $environment );
$environment    = ( new CaptainCore\Environments )->get( $environment_id );

$details                  = json_decode( $environment->details );
$details->quicksave_count = count( $quicksaves );

$environment_update = [
    "environment_id" => $environment_id,
    "details"        => json_encode( $details ),
    "updated_at"     => date("Y-m-d H:i:s"),
];

( new CaptainCore\Environments )->update( $environment_update, [ "environment_id" => $environment_id ] );

// Prepare request to API
$request = [
    'method'  => 'POST',
    'headers' => [ 'Content-Type' => 'application/json' ],
    'body'    => json_encode( [ 
        "command" => "update-environment",
        "site_id" => $site_id,
        "token"   => $configuration->keys->token,
        "data"    => $environment_update,
    ] ),
];

if ( $system->captaincore_dev ) {
    $request['sslverify'] = false;
}

// Post to CaptainCore API
$response = wp_remote_post( $configuration->vars->captaincore_api, $request );