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

$configuration = ( new CaptainCore\Configurations )->get();

if ( ! empty( $field ) ) {
    if ( is_array( $configuration->{$field}) ) {
        echo json_encode( $configuration->{$field} );
        return;
    }
    echo $configuration->{$field};
    return;
}

echo json_encode( $configuration );