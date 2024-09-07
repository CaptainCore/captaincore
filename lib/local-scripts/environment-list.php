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
parse_str( implode( '&', $args ), $arguments );

$site = array_keys($arguments)[0];

if ( empty( $site ) ) {
	echo 'Error: Please specify a <site>.';
	return;
}

if (  ctype_digit( $site ) || is_int( $site )  ) {
    $site_id = $site;
} else {
    $lookup  = ( new CaptainCore\Sites )->where( [ "site" => $site ] );
    $site_id = $lookup[0]->site_id;
    
    // Error if site not found
    if ( count( $lookup ) == 0 ) {
        echo "Error: Site '$site' not found.";
        return;
    }
}

$environments  = ( new CaptainCore\Site( $site_id ) )->environments();
echo strtolower( implode( ' ', array_column ( $environments, "environment" ) ) );
