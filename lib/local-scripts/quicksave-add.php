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

// Fetch site
$site           = ( new CaptainCore\Sites )->get( $lookup[0]->site_id );
$environment_id = ( new CaptainCore\Site( $site->site_id ) )->fetch_environment_id( $environment );
$env            = $environment;
$environment    = ( new CaptainCore\Environments )->get( $environment_id );

// Loads CLI configs
$json = "{$_SERVER['HOME']}/.captaincore/config.json";

if ( ! file_exists( $json ) ) {
	echo "Error: Configuration file not found.";
	return;
}

$config_data = json_decode ( file_get_contents( $json ) );
$system      = $config_data[0]->system;
$path        = $system->path;

foreach($config_data as $config) {
	if ( isset( $config->captain_id ) and $config->captain_id == $captain_id ) {
		$configuration = $config;
		break;
	}
}

if ( $system->captaincore_fleet == true ) {
	$path = "{$path}/{$captain_id}";
}

$time_now  = date("Y-m-d H:i:s");
$site_path = "{$path}/{$site->site}_{$site->site_id}/{$env}/quicksave/";

// Write out new data
file_put_contents( "{$site_path}versions/plugins.json", json_encode( json_decode( $environment->plugins ), JSON_PRETTY_PRINT ) );
file_put_contents( "{$site_path}versions/themes.json", json_encode( json_decode( $environment->themes ), JSON_PRETTY_PRINT ) );
file_put_contents( "{$site_path}versions/core.json", $environment->core );

# New commit
shell_exec( "cd {$site_path}; git add -A" );

# Current git status
$git_status = trim ( shell_exec( "cd {$site_path}; git status -s") );

if ( $git_status == "" and $force != true ) {
	# Skip quicksave as nothing changed
	echo "Quicksave skipped as nothing changed";
	return;
}

# New commit
$git_commit = trim( shell_exec( "cd {$site_path}; git commit -m \"quicksave on {$time_now}\"" ) );

# Save git hash
$git_commit = trim ( shell_exec( "cd {$site_path}; git log -n 1 --pretty=format:\"%H\"" ) );  # Get hash of last commit (commit hash)
$git_status = trim ( shell_exec( "cd {$site_path}; git show {$git_commit} --shortstat --format=" ) );
$core       = trim ( shell_exec( "cd {$site_path}; git show {$git_commit}:versions/core.json" ) );
$themes     = trim ( shell_exec( "cd {$site_path}; git show {$git_commit}:versions/themes.json" ) );
$plugins    = trim ( shell_exec( "cd {$site_path}; git show {$git_commit}:versions/plugins.json" ) );
$timestamp  = trim ( shell_exec( "cd {$site_path}; git show -s --pretty=format:\"%ct\" {$git_commit}" ) ); # Get date of last commit (UNIX timestamp)

$dt         = new DateTime("@$timestamp");  // convert UNIX timestamp to PHP DateTime
$created_at = $dt->format('Y-m-d H:i:s'); // output = 2017-01-01 00:00:00

$quicksave_add = [
	"created_at"     => $created_at,
	"site_id"        => $site->site_id,
	"environment_id" => $environment_id,
	"git_commit"     => $git_commit,
	"git_status"     => $git_status,
	"core"           => $core,
	"themes"         => json_encode( json_decode( $themes ) ),
	"plugins"        => json_encode( json_decode( $plugins ) ),
];

// Update current environment with new data.
$quicksave_add["quicksave_id"] = ( new CaptainCore\Quicksaves )->insert( $quicksave_add );

// Prepare request to API
$request = [
    'method'  => 'POST',
    'headers' => [ 'Content-Type' => 'application/json' ],
    'body'    => json_encode( [ 
        "command" => "quicksave-add",
        "site_id" => $site->site_id,
        "token"   => $configuration->keys->token,
        "data"    => $quicksave_add,
    ] ),
];

if ( ! empty( $system->captaincore_dev ) ) {
    $request['sslverify'] = false;
}

// Post to CaptainCore API
$response = wp_remote_post( $configuration->vars->captaincore_api, $request );
echo $response['body'];

# Generate quicksave usage stats
$quicksave_count = trim( shell_exec( "cd {$site_path}; git rev-list --all --count" ) );

$ostype = PHP_OS;
if ( $ostype == "Linux" ) {
	$quicksave_storage = trim( str_replace ( ".", "", shell_exec( "cd {$site_path}; du -s --block-size=1 ." ) ) );
}

if ( $ostype == "Darwin" ) {	
	$quicksave_storage = trim ( shell_exec( "cd {$site_path}; find . -type f -print0 | xargs -0 stat -f%z | awk '{b+=$1} END {print b}'" ) );
}

$environment              = ( new CaptainCore\Environments )->get( $environment_id );
$details                  = json_decode( $environment->details );
$details->quicksave_usage = [
	"count"          => $quicksave_count,
	"storage"        => $quicksave_storage,
];

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

# Generate capture
shell_exec( "captaincore capture {$site->site}-{$env} --captain-id=$captain_id" );