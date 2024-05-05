<?php

parse_str( implode( '&', $args ), $arguments );
$arguments = (object) $arguments;

if ( is_file( $arguments->file )){
    $data = json_decode( file_get_contents( $arguments->file ) );
    echo implode( " ", array_column( $data, $arguments->field ) );
}