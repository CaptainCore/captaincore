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

// Loads CLI configs
$json = "{$_SERVER['HOME']}/.captaincore-cli/config.json";

if ( ! file_exists( $json ) ) {
	echo "Error: Configuration file not found.";
	return;
}

$config_data = json_decode ( file_get_contents( $json ) );
$system      = $config_data[0]->system;
$path        = $system->path;
$site        = ( new CaptainCore\Sites )->get( $site_id );
$environments = ( new CaptainCore\Site( $site_id ) )->environments();

foreach( $environments as $environment ) {
	$environment_name         = lcfirst( $environment->environment );
	$details                  = ( isset( $environment->details ) ? json_decode( $environment->details ) : (object) [] );
	if ( empty( $details->screenshot_base ) ) {
		echo "No captures found for {$site->site}-{$environment_name}\n";
		continue;
	}
	echo "Fetching latest capture screenshot for {$site->site}-{$environment_name}\n";
	$working_image    = "{$system->rclone_upload_uri}/{$captain_id}/{$site->site}_{$site->site_id}/{$environment_name}/captures/%23_{$details->screenshot_base}.jpg";
	echo shell_exec( "cd {$system->path_tmp}
wget -O {$site->site}_{$site->site_id}_{$details->screenshot_base}.jpg $working_image
convert {$site->site}_{$site->site_id}_{$details->screenshot_base}.jpg -thumbnail 800 -gravity North -crop 800x500+0+0 {$details->screenshot_base}_thumb-800.jpg
convert {$details->screenshot_base}_thumb-800.jpg -thumbnail 100 {$details->screenshot_base}_thumb-100.jpg
rclone move {$details->screenshot_base}_thumb-100.jpg {$system->rclone_upload}{$captain_id}/{$site->site}_{$site->site_id}/{$environment_name}/screenshots/ --fast-list --transfers=32 --no-update-modtime --progress
rclone move {$details->screenshot_base}_thumb-800.jpg {$system->rclone_upload}{$captain_id}/{$site->site}_{$site->site_id}/{$environment_name}/screenshots/ --fast-list --transfers=32 --no-update-modtime --progress
rm {$site->site}_{$site->site_id}_{$details->screenshot_base}.jpg");

}

