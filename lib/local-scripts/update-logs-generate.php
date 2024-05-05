<?php

$captain_id = getenv('CAPTAIN_ID');

// Converts --arguments into $arguments
parse_str( implode( '&', $args ), $arguments );
$arguments = (object) $arguments;

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

if (!function_exists('str_starts_with')) {  // PHP < 8.0
    function str_starts_with(string $haystack, string $needle): bool {
        return strpos($haystack, $needle) === 0;
    }
}

$current             = (object) [];
$previous            = (object) [];
$quicksave_path      = "$system->path/{$arguments->site}_{$arguments->site_id}/{$arguments->environment}/quicksave";
$status              = trim ( shell_exec( "cd $quicksave_path; git diff $arguments->hash_after $arguments->hash_before --shortstat --format=" ) );
$current->core       = trim ( shell_exec( "cd $quicksave_path; git show {$arguments->hash_after}:versions/core.json" ) );
$current->themes     = json_decode( trim ( shell_exec( "cd $quicksave_path; git show {$arguments->hash_after}:versions/themes.json" ) ) );
$current->plugins    = json_decode( trim ( shell_exec( "cd $quicksave_path; git show {$arguments->hash_after}:versions/plugins.json" ) ) );
$current->created_at = trim ( shell_exec( "cd $quicksave_path; git show -s --pretty=format:\"%ct\" {$arguments->hash_after}" ) );

foreach( [ "once" ] as $run ) {
    $previous->created_at = trim ( shell_exec( "cd $quicksave_path; git show -s --pretty=format:\"%ct\" {$arguments->hash_before}" ) );
    $previous->core       = trim ( shell_exec( "cd  $quicksave_path; git show {$arguments->hash_before}:versions/core.json" ) );
    $previous->themes     = json_decode( trim ( shell_exec( "cd  $quicksave_path; git show {$arguments->hash_before}:versions/themes.json" ) ) );
    $previous->plugins    = json_decode( trim ( shell_exec( "cd  $quicksave_path; git show {$arguments->hash_before}:versions/plugins.json" ) ) );

    $themes_names          = array_column( $current->themes, 'name' );
    $plugins_names         = array_column( $current->plugins, 'name' );
    $compare_themes_names  = array_column( $previous->themes, 'name' );
    $compare_plugins_names = array_column( $previous->plugins, 'name' );
    $themes_deleted_names  = array_diff( $compare_themes_names, $themes_names );
    $plugins_deleted_names = array_diff( $compare_plugins_names, $plugins_names );
    $themes_deleted        = [];
    $plugins_deleted       = [];

    foreach( $current->themes as $key => $theme ) {
        $compare_theme_key = null;
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
            }
        }
    }
    foreach( $current->plugins as $key => $plugin ) {
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

$update_log = (object) [
    "created_at"      => $current->created_at,
    "started_at"      => $previous->created_at,
    "hash_before"     => $arguments->hash_before,
    "hash_after"      => $arguments->hash_after,
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

echo json_encode( $update_log, JSON_PRETTY_PRINT );