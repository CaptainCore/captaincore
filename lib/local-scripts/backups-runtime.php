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

foreach($config_data as $config) {
	if ( isset ( $config->captain_id ) and $config->captain_id == $captain_id ) {
		$configuration = $config;
		break;
	}
}

if ( $system->captaincore_fleet == "true" ) {
    $system->path = "{$system->path}/{$captain_id}";
}

function secondsToTime($seconds) {
    $dtF = new \DateTime('@0');
    $dtT = new \DateTime("@$seconds");
    if ( $seconds < 60 ) {
        return $dtF->diff($dtT)->format('%s seconds');
    }
    if ( $seconds < 3600 ) {
        return $dtF->diff($dtT)->format('%i minutes and %s seconds');
    }
    return $dtF->diff($dtT)->format('%h hours, %i minutes and %s seconds');   
}

$runtime  = "{$system->path}/{$arguments->site}_{$arguments->site_id}/{$arguments->environment}/backups/runtime";
$runtimes = file_get_contents( $runtime );
$runtimes = explode("\n", $runtimes);
foreach( $runtimes as $runtime ) {
    $item = explode(" ", $runtime);
    if ( ! empty( $item ) && count ( $item ) == 2 ) {
        $start  = $item[0];
        $finish = $item[1];

        $start_human = new DateTime("@$start");

        $total = $finish - $start;
        echo "{$start_human->format('F jS Y g:i a')} - ". secondsToTime($total) . "\n";
    }
}