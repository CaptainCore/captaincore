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

$arguments = (object) $arguments;

// Loads CLI configs
$json = "{$_SERVER['HOME']}/.captaincore/config.json";

if ( ! file_exists( $json ) ) {
	echo "Error: Configuration file not found.";
	return;
}

$config_data = json_decode ( file_get_contents( $json ) );
$system      = $config_data[0]->system;

if ( $system->captaincore_fleet == "true" ) {
    $system->rclone_backup = "{$system->rclone_backup}/{$captain_id}";
}
$restic_key   = $_SERVER['HOME']. "/.captaincore/data/restic.key";
$command      = "restic ls -l $arguments->backup_id / --recursive --repo rclone:{$system->rclone_backup}/{$arguments->site}_{$arguments->site_id}/{$arguments->environment}/restic-repo --json --password-file=${restic_key}";
$items        = shell_exec( $command );
$items        = explode( PHP_EOL, $items );
$folder_usage = [];
$omit         = false;
$omit_items   = [ "/wp-content/uploads/", "/wp-content/blog.dir/" ];
if ( count ( $items ) > 50000 ) {
    $omit = true;
}
foreach ( $items as $key => $item ) {
    $row = json_decode( $item );
    if ( empty ( $row->path ) ) {
        unset( $items[ $key ] );
        continue;
    }
    if ( $omit && in_array( substr( $row->path, 0, 20 ), $omit_items ) && $row->type == "file" ) {
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

function buildTree( $branches ) {
    // Create a hierarchy where keys are the labels
    $rootChildren = [];
    $omitted      = false;
    foreach($branches as $branch) {
        $children =& $rootChildren;
        $paths = explode( "/", $branch->path );
        foreach( $paths as $label ) {
            if ( $label == "" ) { 
                continue;
            };
            $ext = "";
            if ( strpos( $label, "." ) !== false ) { 
                $ext = substr( $label, strpos( $label, "." ) + 1 );
            }
            if ( empty( $branch->count ) ) {
                $branch->count = 1;
            }
            if ( empty( $branch->size ) ) {
                $branch->size = 0;
            }
            if ( $branch->type == "dir" && $branch->count > 1 ) {
                $omitted = true;
            }
            if (!isset($children[$label])) $children[$label] = [ "//path" => $branch->path, "//type" => $branch->type, "//size" => $branch->size, "//count" => $branch->count, "//ext" => $ext ];
            $children =& $children[$label];
        }
    }
    // Create target structure from that hierarchy
    function recur($children) {
        $result = [];
        foreach( $children as $label => $grandchildren ) {
            $node = [ 
                "name"  => $label,
                "path"  => $grandchildren["//path"],
                "type"  => $grandchildren["//type"],
                "count" => $grandchildren["//count"],
                "size"  => $grandchildren["//size"],
                "ext"   => $grandchildren["//ext"]
            ];
            unset( $grandchildren["//path"] );
            unset( $grandchildren["//type"] );
            unset( $grandchildren["//size"] );
            unset( $grandchildren["//count"] );
            unset( $grandchildren["//ext"] );
            if ( count($grandchildren) ) { 
                $node["children"] = recur( $grandchildren );
            };
            $result[] = $node;
        }
        return $result;
    }
    return [ $omitted, recur($rootChildren) ];
}

$results = buildTree( $items );

function sortRecurse(&$array) {
    usort($array, fn($a, $b) => [$a['type'], $a['name']] <=> [$b['type'], $b['name']]);
    foreach ($array as &$subarray) {
        if ( isset( $subarray['children']) ) {
            sortRecurse($subarray['children']);
        }
    }
    return $array;
}

echo json_encode( [ 
    "omitted" => $results[0],
    "files"   => sortRecurse( $results[1] ),
]);