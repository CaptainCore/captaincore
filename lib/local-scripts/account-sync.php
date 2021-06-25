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

// Prepare request to API
$request = [
    'method'  => 'POST',
    'headers' => [ 'Content-Type' => 'application/json' ],
    'body'    => json_encode( [ 
        "command"    => "account-get-raw",
        "account_id" => $account_id,
        "token"      => $configuration->keys->token,
    ] ),
];

if ( ! empty( $system->captaincore_dev ) ) {
    $request['sslverify'] = false;
}

// Post to CaptainCore API
$response = wp_remote_post( $configuration->vars->captaincore_api, $request );
if ( is_wp_error( $response ) ) {
    $error_message = $response->get_error_message();
    return "Something went wrong: $error_message";
}

$results = json_decode( $response['body'] );

if ( $debug !== null ) {
    echo json_encode( $results, JSON_PRETTY_PRINT );
    return;
}

$account       = $results->account;
$account_check = ( new CaptainCore\Accounts )->get( $account->account_id );

$domains = $account->domains;
$sites   = $account->sites;
$users   = $account->users;
unset( $account->domains );
unset( $account->sites );
unset( $account->users );

if ( empty( $account_check ) ) {
    // Insert new account
    ( new CaptainCore\Accounts )->insert( (array) $account );
    echo "Inserting account #{$account->account_id}\n";
} else {
    // update new account
    ( new CaptainCore\Accounts )->update( (array) $account, [ "account_id" => $account->account_id ] );
    echo "Updating account #{$account->account_id}\n";
}
foreach ( $domains as $account_domain ) {
    $account_domain_check = ( new CaptainCore\AccountDomain )->get( $account_domain->account_domain_id );
    // Insert new record
    if ( empty( $account_domain_check ) ) {
        ( new CaptainCore\AccountDomain )->insert( (array) $account_domain );
        echo "Inserting account_domain #{$account_domain->account_domain_id}\n";
        continue;
    }
    // Update existing record
    ( new CaptainCore\AccountDomain )->update( (array) $account_domain, [ "account_domain_id" => $account_domain->account_domain_id ] );
    echo "Updating account_domain #{$account_domain->account_domain_id}\n";
}

foreach ( $sites as $account_site ) {
    $account_site_check = ( new CaptainCore\AccountSite )->get( $account_site->account_site_id );
    // Insert new record
    if ( empty( $account_site_check ) ) {
        ( new CaptainCore\AccountSite )->insert( (array) $account_site );
        echo "Inserting account_site #{$account_site->account_site_id}\n";
        continue;
    }
    // Update existing record
    ( new CaptainCore\AccountSite )->update( (array) $account_site, [ "account_site_id" => $account_site_check->account_site_id ] );
    echo "Updating account_site #{$account_site->account_site_id}\n";
}

foreach ( $users as $account_user ) {
    $account_user_check = ( new CaptainCore\AccountUser )->get( $account_user->account_user_id );
    // Insert new record
    if ( empty( $account_user_check ) ) {
        ( new CaptainCore\AccountUser )->insert( (array) $account_user );
        echo "Inserting account_user #{$account_user->account_user_id}\n";
        continue;
    }
    // Update existing record
    ( new CaptainCore\AccountUser )->update( (array) $account_user, [ "account_user_id" => $account_user->account_user_id ] );
    echo "Updating account_user #{$account_user->account_user_id}\n";
}