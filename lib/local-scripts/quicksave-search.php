<?php

$captain_id = getenv('CAPTAIN_ID');

$site = $args[0];

if ( base64_encode(base64_decode($args[1], true)) === $args[1]){
    $args[1] = base64_decode( $args[1] );
 }

$search = explode( ":", $args[1], 3 );
$search_type = $search[0] ."s";
$search_field = $search[1];
$search_value = $search[2];

if( strpos( $site, "-" ) !== false ) {
	$split       = explode( "-", $site );
	$site        = $split[0];
	$environment = $split[1];
}

if( strpos( $site, "@" ) !== false ) {
	$split       = explode( "@", $site );
	$site        = $split[0];
	$provider    = $split[1];
}

if( isset( $environment ) && strpos( $environment, "@" ) !== false ) {
	$split       = explode( "@", $environment );
	$environment = $split[0];
	$provider    = $split[1];
}

// Assign default format to JSON
if ( empty( $format ) ) {
	$format = "json";
}
foreach( [ "once" ] as $run ) {
	if ( ! empty( $provider ) ) {
		$lookup = ( new CaptainCore\Sites )->where( [ "site" => $site, "provider" => $provider, "status" => "active" ] );
		continue;
	}
	if ( ctype_digit( $site ) ) {
		$lookup = ( new CaptainCore\Sites )->where( [ "site_id" => $site, "status" => "active" ] );
		continue;
	}
	$lookup = ( new CaptainCore\Sites )->where( [ "site" => $site, "status" => "active" ] );
}

// Error if site not found
if ( count( $lookup ) == 0 ) {
	return "";
}

// Fetch site
$site = ( new CaptainCore\Site( $lookup[0]->site_id ) )->get();

// Set environment if not defined
if ( empty( $environment ) ) {
	$environment = "production";
}

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

$command    = "cd $system->path/{$site->site}_{$site->site_id}/{$environment}/quicksave/; echo -n \"$( git log --pretty='format:%H %ct' )\"";
$response   = shell_exec( $command );
$quicksaves = explode( "\n", $response );
foreach ( $quicksaves as $key => $quicksave ) {
    $split            = explode( " ", $quicksave );
    if ( empty( $split[0] ) || $split[0] == "-n" || empty( $split[1] ) ) {
        unset(  $quicksaves[$key] );
        continue;
    }
    $quicksave_item = (object) [ 
        "hash"       => $split[0],
        "created_at" => $split[1]
    ];
    if ( is_file( "$system->path/{$site->site}_{$site->site_id}/{$environment}/quicksaves/commit-{$split[0]}.json" ) ) {
        $quicksave_data = json_decode( file_get_contents( "$system->path/{$site->site}_{$site->site_id}/{$environment}/quicksaves/commit-{$split[0]}.json" ) );
        $items = $quicksave_data->{$search_type};
        if ( empty( $items ) ) {
            # hash is empty, skip to next
            unset(  $quicksaves[$key] );
            continue;
        }
        foreach ( $items as $item ) {
            if ( isset( $item->{$search_field} ) && $search_value == $item->{$search_field} ) {
                unset( $item->changed_version );
                unset( $item->changed_title );
                $quicksave_item->item = $item;
                continue;
            }
        }
        if ( empty( $quicksave_item->item ) ) {
            $quicksave_item->item = "";
        }
        $quicksaves[$key] = $quicksave_item;
    }
    
}

$quicksaves = array_values($quicksaves);
usort($quicksaves, fn($a, $b) => $a->created_at < $b->created_at);
$previous = -1;
foreach( $quicksaves as $key => $quicksave ) {
    if ( empty( $quicksaves[ $previous ] ) ) {
        $previous = $key;
        continue;
    }
    if ( $quicksave->item == $quicksaves[ $previous ]->item ) {
        unset( $quicksaves[ $key ] );
        continue;
    }
    $previous = $key;
}

if ( count( $quicksaves ) == 1 ) {
    echo "[]";
    return;
}

echo json_encode( array_values($quicksaves), JSON_PRETTY_PRINT );
