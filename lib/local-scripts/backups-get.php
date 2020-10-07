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
    $system->rclone_backup = "{$system->rclone_backup}/{$captain_id}";
}
$command      = "restic ls -l $backup_id / --recursive --repo rclone:{$system->rclone_backup}/${site}_${site_id}/${environment}/restic-repo --json --password-file=\"{$_SERVER['HOME']}/.captaincore-cli/data/restic.key\"";
$items        = shell_exec( $command );
$items        = explode( PHP_EOL, $items );
$folder_usage = [];
if ( count ( $items ) > 50000 ) {
    $omit = true;
}
foreach ( $items as $key => $item ) {
    $row = json_decode( $item );
    if ( empty ( $row->path ) ) {
        unset( $items[ $key ] );
        continue;
    }
    if ( $omit && substr( $row->path, 0, 20 ) == "/wp-content/uploads/" && $row->type == "file" ) {
        unset( $items[ $key ] );
        $path = dirname( $row->path );
        if ( empty( $folder_usage[ $path ] ) ) {
            $folder_usage[ $path ] = (object) [ "folder_size" => 0, "folder_count" => 0 ];
        }
        $folder_usage[ $path ]->folder_size  = $folder_usage[ $path ]->folder_size + $row->size;
        $folder_usage[ $path ]->folder_count = $folder_usage[ $path ]->folder_count + 1;
        continue;
    }
    unset( $row->mtime );
    unset( $row->atime );
    unset( $row->ctime );
    unset( $row->struct_type );
    unset( $row->mode );
    unset( $row->uid );
    unset( $row->gid );
    $items[ $key ] = $row;
}

foreach ( $folder_usage as $key => $value ) {
    foreach( $items as $item) {
        if ( $key == $item->path ) {
            $item->size  = $value->folder_size;
            $item->count = $value->folder_count;
            break;
        }
    }
}

foreach ( $items as $key => $item ) {
    $items[ $key ] = json_encode( $item );
}

echo implode( "\n", $items );