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
parse_str( implode( '&', $args ), $arguments );
$arguments   = (object) $arguments;
$hash        = $arguments->hash;
$environment = $arguments->environment;
$site        = $arguments->site;
$site_id     = $arguments->site_id;

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
$current          = (object) [];
$previous         = (object) [];
$quicksave_path   = "$system->path/{$site}_{$site_id}/{$environment}/quicksave";
$status           = trim ( shell_exec( "cd $quicksave_path; git show $hash --shortstat --format=" ) );
$current->core    = trim ( shell_exec( "cd $quicksave_path; git show {$hash}:versions/core.json" ) );
$current->themes  = json_decode( trim ( shell_exec( "cd $quicksave_path; git show {$hash}:versions/themes.json" ) ) );
$current->plugins = json_decode( trim ( shell_exec( "cd $quicksave_path; git show {$hash}:versions/plugins.json" ) ) );

if ( empty( $previous_hash ) ) {
    $previous_hash = trim ( shell_exec( "cd $quicksave_path; git show -s --pretty=format:\"%P\" $hash" ) );
}

$files_changed    = trim ( shell_exec( "cd $quicksave_path; git diff $previous_hash $hash --name-only" ) );
$files_changed    = explode( "\n", $files_changed );

if (!function_exists('str_starts_with')) {  // PHP < 8.0
    function str_starts_with(string $haystack, string $needle): bool {
        return strpos($haystack, $needle) === 0;
    }
}

foreach( [ "once" ] as $run ) {
    if ( $previous_hash == "" ) { 
        continue;
    }
    $previous->created_at = trim ( shell_exec( "cd $quicksave_path; git show -s --pretty=format:\"%ct\" {$previous_hash}" ) );
    $previous->core       = trim ( shell_exec( "cd $quicksave_path; git show {$previous_hash}:versions/core.json" ) );
    $previous->themes     = json_decode( trim ( shell_exec( "cd $quicksave_path; git show {$previous_hash}:versions/themes.json" ) ) );
    $previous->plugins    = json_decode( trim ( shell_exec( "cd $quicksave_path; git show {$previous_hash}:versions/plugins.json" ) ) );

    $themes_names          = array_column( $current->themes, 'name' );
    $plugins_names         = array_column( $current->plugins, 'name' );
    $compare_themes_names  = array_column( $previous->themes, 'name' );
    $compare_plugins_names = array_column( $previous->plugins, 'name' );
    $themes_deleted_names  = array_diff( $compare_themes_names, $themes_names );
    $plugins_deleted_names = array_diff( $compare_plugins_names, $plugins_names );
    $themes_deleted        = [];
    $plugins_deleted       = [];

    foreach( $current->themes as $key => $theme ) {
        if ( ! in_array( $theme->name, $compare_themes_names ) ) {
            $current->themes[ $key ]->changed = true;
            $current->themes[ $key ]->new     = true;
        }
        foreach( $previous->themes as $previous_theme ) {
            if ( $theme->name == $previous_theme->name ) {
                $current->themes[ $key ]->changed = false;
                if ( $theme->version != $previous_theme->version ) {
                    $current->themes[ $key ]->changed_version = $previous_theme->version;
                    $current->themes[ $key ]->changed = true;
                }
                if ( $theme->status != $previous_theme->status ) {
                    $current->themes[ $key ]->changed_status = $previous_theme->status;
                    $current->themes[ $key ]->changed = true;
                }
                if ( $theme->title != $previous_theme->title ) {
                    $current->themes[ $key ]->changed_title = $previous_theme->title;
                    $current->themes[ $key ]->changed = true;
                }
                if ( $current->themes[ $key ]->changed ) {
                    continue;
                }
                foreach( $files_changed as $file ) {
                    if ( ! str_starts_with($file, "themes/" ) ) {
                        continue;
                    }
                    if ( str_starts_with( $file, "themes/{$theme->name}" ) ) {
                        $current->themes[ $key ]->changed = true;
                    }
                }
            }
        }
    }
    foreach( $current->plugins as $key => $plugin ) {
        if ( ! in_array( $plugin->name, $compare_plugins_names ) ) {
            $current->plugins[ $key ]->changed = true;
            $current->plugins[ $key ]->new     = true;
        }
        foreach( $previous->plugins as $previous_plugin ) {
            if ( $plugin->name == $previous_plugin->name ) {
                $current->plugins[ $key ]->changed = false;
                if ( $plugin->version != $previous_plugin->version ) {
                    $current->plugins[ $key ]->changed_version = $previous_plugin->version;
                    $current->plugins[ $key ]->changed = true;
                }
                if ( $plugin->status != $previous_plugin->status ) {
                    $current->plugins[ $key ]->changed_status = $previous_plugin->status;
                    $current->plugins[ $key ]->changed = true;
                }
                if ( $plugin->title != $previous_plugin->title ) {
                    $current->plugins[ $key ]->changed_title = $previous_plugin->title;
                    $current->plugins[ $key ]->changed = true;
                }
                if ( $current->plugins[ $key ]->changed ) {
                    continue;
                }
                foreach( $files_changed as $file ) {
                    if ( ! str_starts_with($file, "plugins/" ) ) {
                        continue;
                    }
                    if ( str_starts_with( $file, "plugins/{$plugin->name}" ) ) {
                        $current->plugins[ $key ]->changed = true;
                    }
                }
            }
        }
    }

    // Attached removed themes
    foreach ( $themes_deleted_names as $theme ) {
        $key              = array_search( $theme, array_column( $previous->themes ,'name' ) );
        $deleted          = $previous->themes[ $key ];
        $themes_deleted[] = $deleted;
    }

    foreach ( $plugins_deleted_names as $plugin ) {
        $key               = array_search( $plugin, array_column( $previous->plugins ,'name' ) );
        $deleted           = $previous->plugins[ $key ];
        $plugins_deleted[] = $deleted;
    }

}

usort( $current->themes, function($a, $b) {
    $diff = $b->changed <=> $a->changed;
    return ($diff !== 0) ? $diff : $a->name <=> $b->name;
});

usort( $current->plugins, function($a, $b) {
    $diff = $b->changed <=> $a->changed;
    return ($diff !== 0) ? $diff : $a->name <=> $b->name;
});

$quicksave = (object) [
    "core"            => $current->core,
    "core_previous"   => $previous->core,
    "theme_count"     => count( $current->themes),
    "themes"          => $current->themes,
    "themes_deleted"  => $themes_deleted,
    "plugin_count"    => count( $current->plugins),
    "plugins"         => $current->plugins,
    "plugins_deleted" => $plugins_deleted,
    "status"          => $status,
];

if ( $previous->created_at ) {
    $quicksave->previous_created_at = $previous->created_at;
}

echo json_encode( $quicksave, JSON_PRETTY_PRINT );
exit;

$command    = 
$response   = shell_exec( $command );
$quicksaves = explode( "\n", $response );
foreach ( $quicksaves as $key => $quicksave ) {
    $split     = explode( " ", $quicksave );
    $quicksaves[$key] = (object) [ 
        "hash"       => $split[0],
        "created_at" => $split[1]
    ];
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