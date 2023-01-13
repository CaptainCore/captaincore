<?php

$COLOR_RED    = "\033[31m";
$COLOR_NORMAL = "\033[39m";
$site         = $args[0];
$captain_id   = getenv('CAPTAIN_ID');

// Replaces dashes in keys with underscores
foreach( $args as $index => $arg ) {
	$split = strpos( $arg, "=" );
	if ( $split ) {
		$key   = str_replace('-', '_', substr( $arg , 0, $split ) );
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

if ( ! empty( $arguments ) ) {
    $pass_through_args = false;
    $additional_args   = [];
    $arguments         = base64_decode( $arguments );
    preg_match_all('/(--[^\s]+="[^"]+")|"([^"]+)"|\'([^\']+)\'|([^\s]+)/', $arguments, $matches );
    $arguments         = $matches[0];
    $site              = $arguments[0];
    unset( $arguments[0] );
    foreach( $arguments as $key => $argument ) {
        if ( empty( $argument ) ) {
            continue;
        }
        if ( substr( $argument, 0, 10 ) === "--command=" ) {
            $command = substr( $argument, 10, strlen( $argument ) );
            $pass_through_args = true;
        }
        if ( substr( $argument, 0, 9 ) === "--script=" ) {
            $script  = substr( $argument, 9, strlen( $argument ) );
            $pass_through_args = true;
        }
        if ( substr( $argument, 0, 9 ) === "--recipe=" ) {
            $recipe  = substr( $argument, 9, strlen( $argument ) );
            $pass_through_args = true;
        }
        if (strpos($argument, '=') !== false) {
            $position = strpos( $argument, '=' );
            $argument = substr_replace( $argument, "=\\\"", $position, 1 );
            $argument = "$argument\\\"";
            $arguments[$key] = $argument;
        }
    }
    if ( $pass_through_args == true ) {
        $additional_args = implode( " ", $arguments );
    }
}

$command = trim( $command, '"' );

// Loads CLI configs
$json = "{$_SERVER['HOME']}/.captaincore/config.json";

if ( ! file_exists( $json ) ) {
	echo "Error: Configuration file not found.";
	return;
}

$config_data = json_decode ( file_get_contents( $json ) );
$system      = $config_data[0]->system;
$path        = $system->path;

if ( strpos($site, '@') !== false ) {
    $parts    = explode( "@", $site );
    $site     = $parts[0];
    $provider = $parts[1];
}

// Determines environment
if ( strpos($site, '-staging') !== false ) {
    $site        = str_replace( "-staging", "", $site );
    $environment = "staging";
} else {
    $site        = str_replace( "-production", "", $site );
    $environment = "production";
}

if ( empty( $site ) ) {
    echo "${COLOR_RED}Error:${COLOR_NORMAL} Please specify a <site>.\n";
    return;
}

foreach( [ "once" ] as $run ) {
	if ( $provider ) {
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
	echo "${COLOR_RED}Error:${COLOR_NORMAL} Site '$site' not found.\n";
	return;
}

// Fetch site
$site           = ( new CaptainCore\Sites )->get( $lookup[0]->site_id );
$environment_id = ( new CaptainCore\Site( $site->site_id ) )->fetch_environment_id( $environment );
$environment    = ( new CaptainCore\Environments )->get( $environment_id );

if ( empty( $environment ) ) {
    echo "${COLOR_RED}Error:${COLOR_NORMAL} Environment $environment->environment not found for '$site->name'.\n";
	return;
}

if ( empty( $environment->address ) ) {
    echo "${COLOR_RED}Error:${COLOR_NORMAL} Environment $environment->environment not found for '$site->name'.\n";
    return;
}

if ( $environment->protocol != "sftp" ) {
    echo "${COLOR_RED}Error:${COLOR_NORMAL} SSH not supported (Protocol is $environment->protocol).";
    return;
}

if ( $environment->provider != "kinsta" && $environment->address == "" ) {
    echo "${COLOR_RED}Error:${COLOR_NORMAL} Missing Kinsta site.";
    return;
}

if ( $environment->provider == "wpengine" && $environment == "staging" ) {
    echo "${COLOR_RED}Error:${COLOR_NORMAL} Missing WP Engine site.";
    return;
}

if ( $environment->provider == "wpengine" && $environment == "staging" ) {
    echo "${COLOR_RED}Error:${COLOR_NORMAL} Missing WP Engine site.";
    return;
}

$site_data        = json_decode( $site->details );
$remote_options   = "-q -oStrictHostKeyChecking=no";
$environment_vars = "";

if ( is_array( $site_data->environment_vars ) ) { 
	foreach ( $site_data->environment_vars as $item ) { 
		$environment_vars = "export {$item->key}='{$item->value}' && $environment_vars";
	}
	//$environment_vars = "$environment_vars &&";
}

if ( $site_data->key != 'use_password' && $site_data->key != "" ) {
    $key            = $site_data->key;
}

if ( $site_data->key != 'use_password' && $site_data->key == "" ) {
    $configurations = ( new CaptainCore\Configurations )->get();
	$key            = $configurations->default_key;
}

if ( $site_data->key != 'use_password' ) {
    $remote_options = "$remote_options -oPreferredAuthentications=publickey -i $system->path_keys/${captain_id}/${key}";
}

if ( $site_data->key == 'use_password' ) {
    $before_ssh = "sshpass -p '{$environment->password}'";
}

if ( $site->provider == "kinsta" ) {
    $command_prep  = "$environment_vars cd public/ &&";
    $remote_server = "$remote_options $environment->username@$environment->address -p $environment->port";
}

if ( $site->provider == "wpengine" ) {
    $command_prep  = "$environment_vars rm ~/.wp-cli/config.yml; cd sites/* &&"; 
    $remote_server = "$remote_options {$site->site}@{$site->site}.ssh.wpengine.net";
}

if ( $site->provider == "rocketdotnet" ) {
    $command_prep  = "$environment_vars cd $site->home_directory/ &&";
    $remote_server = "$remote_options $environment->username@$environment->address -p $environment->port";
}

if ( empty( $site->provider ) ) {
    $command_prep  = "$environment_vars cd $environment->home_directory/ &&";
    $remote_server = "$remote_options $environment->username@$environment->address -p $environment->port";
}

if ( ! empty( $command ) ) {
    echo "$before_ssh ssh $remote_server \"$command_prep {$command}\" || captaincore site ssh-fail $site->site --captain-id=$captain_id";
    return;
}

if ( ! empty( $script ) ) {

    if ( file_exists( "$script" ) ) {
      # Pass all arguments found after --script=<script> argument into remote script
      echo "$before_ssh ssh $remote_server \"$command_prep bash -s -- --site=$site->site $additional_args\" < $script || captaincore site ssh-fail $site->site --captain-id=$captain_id";
      return;
    }

    # Not found so attempt to run a local script
    $script_file = "{$_SERVER['HOME']}/.captaincore/lib/remote-scripts/$script";
    if ( ! file_exists( "$script_file" ) ) {
        echo "Error: Can't locate script $script";
        return;
    }

    echo "$before_ssh ssh $remote_server \"$command_prep bash -s -- --site=$site->site $additional_args\" < $script_file || captaincore site ssh-fail $site->site --captain-id=$captain_id";
    return; 
}

if ( ! empty( $recipe ) ) {

    if ( file_exists( "$recipe" ) ) {
      # Pass all arguments found after --script=<script> argument into remote script
      echo "$before_ssh ssh $remote_server \"$command_prep bash -s -- --site=$site->site $additional_args\" < $recipe || captaincore site ssh-fail $site->site --captain-id=$captain_id";
      return;
    }

    # Not found so attempt to run a local script
    $recipe_file = "{$system->path_recipes}/{$captain_id}-{$recipe}.sh";
    if ( ! file_exists( "$recipe_file" ) ) {
        echo "Error: Can't locate recipe $recipe";
        return;
    }

    echo "$before_ssh ssh $remote_server \"$command_prep bash -s -- --site=$site->site $additional_args\" < $recipe_file || captaincore site ssh-fail $site->site --captain-id=$captain_id";
    return; 
}

echo "$before_ssh ssh $remote_server";