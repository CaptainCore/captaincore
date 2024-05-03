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

$command     = "cd $system->path/{$site}_{$site_id}/{$environment}/update-logs/; find ./ -name 'log-*' -type f";
$response    = shell_exec( $command );
$update_logs = explode( "\n", trim( $response ) );
foreach ( $update_logs as $key => $update_log ) {
    $file_name = str_replace( "./", "", $update_log );
    $file_name = str_replace( ".json", "", $update_log );
    if ( ! is_file( "$system->path/{$site}_{$site_id}/{$environment}/update-logs/$file_name.json" ) ) {
        unset( $update_logs[$key]);
        continue;
    }
    $update_log_data = json_decode( file_get_contents( "$system->path/{$site}_{$site_id}/{$environment}/update-logs/$file_name.json" ) );
    
    $update_log_item = (object) [ 
        "hash_before" => $update_log_data->hash_before,
        "hash_after"  => $update_log_data->hash_after,
        "created_at"  => $update_log_data->created_at,
        "started_at"  => $update_log_data->started_at,
        "status"      => $update_log_data->status,
    ];
    if ( $update_log_data->core ) {
        $update_log_item->core = $update_log_data->core;
    }
    if ( $update_log_data->theme_count ) {
        $update_log_item->theme_count = $update_log_data->theme_count;
    }
    if ( $update_log_data->plugin_count ) {
        $update_log_item->plugin_count = $update_log_data->plugin_count;
    }
    if ( $update_log_data->core ) {
        $update_log_item->core = $update_log_data->core;
    }
    if ( $update_log_data->core_previous ) {
        $update_log_item->core_previous = $update_log_data->core_previous;
    }
    $themes_changed  = 0;
    $plugins_changed = 0;
    foreach( $update_log_data->plugins as $plugin ) {
        if ( ! empty( $plugin->changed_version ) || ! empty( $plugin->changed_status ) ){
            $plugins_changed++;
        }
    }
    foreach( $update_log_data->themes as $theme ) {
        if ( ! empty( $theme->changed_version ) || ! empty( $theme->changed_status ) ){
            $themes_changed++;
        }
    }
    $update_log_item->themes_changed  = $themes_changed;
    $update_log_item->plugins_changed = $plugins_changed;
    $update_logs[$key]                = $update_log_item;
}

usort($update_logs, fn($a, $b) => strcmp($b->created_at, $a->created_at));

echo json_encode( $update_logs, JSON_PRETTY_PRINT );

/*
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

*/