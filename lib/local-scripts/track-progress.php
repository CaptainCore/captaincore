<?php
$data    = file_get_contents ( $argv[1] );
$data    = explode( PHP_EOL, $data );
$total   = (int) $data[0];
$current = strlen( $data[1] );
$percent = $current / $total;

$percent_friendly = number_format( $percent * 100, 2 ) . '%';

echo "$percent_friendly - $current / $total completed\n";