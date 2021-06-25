<?php

$captain_id = getenv('CAPTAIN_ID');
$site       = $args[0];

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

if( strpos( $environment, "@" ) !== false ) {
	$split       = explode( "@", $environment );
	$environment = $split[0];
	$provider    = $split[1];
}

// Loads CLI configs
$json = "{$_SERVER['HOME']}/.captaincore/config.json";

if ( ! file_exists( $json ) ) {
	echo "Error: Configuration file not found.";
	return;
}

$config_data = json_decode ( file_get_contents( $json ) );
$system      = $config_data[0]->system;
$path_tmp    = $system->path_tmp;

// Set environment if not defined
if ( $environment == "" ) {
	$environment = "production";
}

$lookup     = ( new CaptainCore\Sites )->where( [ "site" => $site ] );
if ( $provider ) {
	$lookup = ( new CaptainCore\Sites )->where( [ "site" => $site, "provider" => $provider ] );
}


if (count( $lookup ) == 0 && is_numeric( $site ) ) {
    $lookup     = ( new CaptainCore\Sites )->where( [ "site_id" => $site ] );
    if ( $provider ) {
        $lookup = ( new CaptainCore\Sites )->where( [ "site_id" => $site, "provider" => $provider ] );
    }
}

// Error if site not found
if ( count( $lookup ) == 0 ) {
	return "";
}

// Fetch site
$accounts          = ( new CaptainCore\Site( $lookup[0]->site_id ) )->shared_with();
$recipe_ids        = [];
$deployment_script = "";

// Add global defaults
$defaults = ( new CaptainCore\Defaults )->get();

foreach( [ "once" ] as $run ) {

    $deployment_script .= "# Global Defaults\n";

    if ( ! empty( $defaults->timezone ) ) {
        $deployment_script .= "wp option set timezone_string {$defaults->timezone}\n";
    }
    if ( ! empty( $defaults->email ) ) {
        $deployment_script .= "wp option set admin_email {$defaults->email}\n";
    }

    $deployment_script .= "\n";

    if ( ! is_array( $defaults->recipes ) ) {
        continue;
    }
    foreach ( $defaults->recipes as $recipe_id ) {
        $recipe_ids[] = $recipe_id;
    }

}

// Add account defaults
if ( ! isset( $global_only ) ) {
foreach ( $accounts as $account ) {

    $defaults           = json_decode( $account->defaults );
    $deployment_script .= "# Defaults for account: '{$account->name}'\n";

    if ( ! empty( $defaults->timezone ) ) {
        $deployment_script .= "wp option set timezone_string {$defaults->timezone}\n";
    }
    if ( ! empty( $defaults->email ) ) {
        $deployment_script .= "wp option set admin_email {$defaults->email}\n";
    }
    foreach( $defaults->users as $user ) {
        $deployment_script .= "wp user create {$user->username} {$user->email} --role={$user->role} --first_name='{$user->first_name}' --last_name='{$user->last_name}' --send-email\n";
    }

    $deployment_script .= "\n";

    // Default recipes. Verify is array otherwise skip
    if ( ! is_array( $defaults->recipes ) ) {
        continue;
    }
    foreach ( $defaults->recipes as $recipe_id ) {
        $recipe_ids[] = $recipe_id;
    }

}
}
$recipe_ids             = array_unique( $recipe_ids );
$timestamp              = date('Y-m-d-h-m-s');
$token                  = bin2hex( openssl_random_pseudo_bytes( 16 ) );
$deployment_script_file = "$path_tmp/{$captain_id}-{$timestamp}-{$token}.sh";

file_put_contents( $deployment_script_file, $deployment_script );

if ( isset( $debug ) ) {
    echo "captaincore ssh ${site}-${environment} --script=$deployment_script_file --captain-id=$captain_id\n";
    foreach ( $recipe_ids as $recipe_id ) {
        echo "captaincore ssh ${site}-${environment} --recipe=$recipe_id --captain-id=$captain_id\n";
    }
    exit;
}
echo "Deploying default configurations\n";
echo shell_exec( "captaincore ssh ${site}-${environment} --script=$deployment_script_file --captain-id=$captain_id" );

foreach ( $recipe_ids as $recipe_id ) {
    $recipe = ( new CaptainCore\Recipes )->get( $recipe_id );
    echo "Deploying recipe '{$recipe->title}'\n";
    echo shell_exec( "captaincore ssh ${site}-${environment} --recipe=$recipe_id --captain-id=$captain_id" );
}
