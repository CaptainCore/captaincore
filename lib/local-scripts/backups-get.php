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

if ( $system->captaincore_fleet == "true" ) {
    $system->rclone_backup   = "{$system->rclone_backup}/{$captain_id}";
}
$command = "restic ls -l $backup_id / --recursive --repo rclone:{$system->rclone_backup}/${site}_${site_id}/${environment}/restic-repo --json";
$items   = shell_exec( $command );
$items   = explode( PHP_EOL, $items );
foreach ( $items as $key => $item ) {
    $row = json_decode( $item );
    if ( empty ( $row->path ) ) {
        unset( $items[ $key ] );
        continue;
    }
    unset( $row->mtime );
    unset( $row->atime );
    unset( $row->ctime );
    unset( $row->struct_type );
    unset( $row->mode );
    unset( $row->uid );
    unset( $row->gid );
    echo json_encode( $row ) . "\n";
}