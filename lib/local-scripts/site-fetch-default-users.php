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

$lookup  = ( new CaptainCore\Sites )->where( [ "site" => $site ] );
if ( $provider ) {
	$lookup  = ( new CaptainCore\Sites )->where( [ "site" => $site, "provider" => $provider ] );
}

// Error if site not found
if ( count( $lookup ) == 0 ) {
	return "";
}

// Fetch site
$accounts = ( new CaptainCore\Site( $lookup[0]->site_id ) )->shared_with();
foreach ( $accounts as $account_id ) {

    $account  = ( new CaptainCore\Accounts )->get( $account_id );
    $defaults = json_decode( $account->defaults );

    // Output WP-CLI commands
    foreach( $defaults->users as $user ) {
        echo "wp user create {$user->username} {$user->email} --role={$user->role} --first_name='{$user->first_name}' --last_name='{$user->last_name}' --send-email\n";
    }
}
