<?php

$captain_id = getenv('CAPTAIN_ID');
$recipe     = $args[0];

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
$json = "{$_SERVER['HOME']}/.captaincore/config.json";

if ( ! file_exists( $json ) ) {
	echo "Error: Configuration file not found.";
	return;
}

$config_data  = json_decode ( file_get_contents( $json ) );
$system       = $config_data[0]->system;
$path_recipes = $system->path_recipes;

foreach($config_data as $config) {
	if ( isset( $config->captain_id ) and $config->captain_id == $captain_id ) {
		$configuration = $config;
		break;
	}
}

if ( $system->captaincore_fleet == true ) {
	$path = "{$path}/{$captain_id}";
}

if ( $format == 'base64' ) {
	$recipe       = json_decode( base64_decode( $recipe ) ) ;
	$recipe_check = ( new CaptainCore\Recipes )->get( $recipe->recipe_id );
	if ( empty( $recipe_check ) ) {
		// Insert new site
        ( new CaptainCore\Recipes )->insert( (array) $recipe );
        return;
	}
    // update new site
    ( new CaptainCore\Recipes )->update( (array) $recipe, [ "recipe_id" => $recipe->recipe_id ] );

    $recipe_file = "$path_recipes/{$captain_id}-{$recipe->recipe_id}.sh";
    echo "Generating $recipe_file\n";
    file_put_contents( $recipe_file, $recipe->content );
}