#!/usr/bin/env php
<?php

unset( $argv[0] );
$commands = implode( " ", $argv );

// Regex to parse command line https://regexr.com/4154h
$pattern = '/(--[^\s]+="[^"]+")|"([^"]+)"|\'([^\']+)\'|([^\s]+)/';
preg_match_all( $pattern, $commands, $matches );
$args    = $matches[0];
$command = [];

foreach( $args as $index => $argument ) {
	if ( $argument == "--progress" || strpos( $argument, '--captain_id=' ) !== false || strpos( $argument, '--process_id=' ) !== false ) {
		continue;
	}
	$command[] = $argument;
}

// Replaces dashes in keys with underscores as PHP can't assign variables to $-- but $__ works fine.
foreach( $args as $index => $arg ) {
	$split = strpos( $arg, "=" );
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
	if ( substr( $arg, 0, 2 ) === "--" ) {
		continue;
	}
}

// Converts --arguments into $arguments
parse_str( implode( '&', $args ) );

$captain_id = $__captain_id;
$process_id = $__process_id;
$command    = implode( " ", $command );

if ( $process_id == "" or $command == "" ) {
	return;
}

// Loads CLI configs
$json = "{$_SERVER['HOME']}/.captaincore/config.json";

if ( ! file_exists( $json ) ) {
	echo "Error: Configuration file not found.";
	return;
}

$config_data = json_decode ( file_get_contents( $json ) );
$system      = $config_data[0]->system;
$path        = $system->path;
$now         = date( 'U' );

if ( $system->captaincore_fleet == "true" ) {
    $system->path = "{$system->path}/${captain_id}";
}

$running_json = "{$system->path}/running.json";
if ( ! file_exists( $running_json ) ) {
	file_put_contents( $running_json, "[]" );
}

$processes = json_decode( file_get_contents( $running_json ) );
foreach ( $processes as $key => $process ) {
    if ( $process->process_id == $process_id ) {
        $process->completed_at = $now;
    }
}

file_put_contents( $running_json, json_encode( $processes, JSON_PRETTY_PRINT ) );

$process_log = "{$system->path}/process-$process_id-progress.log";
if ( file_exists( $process_log  ) ) {
	$data    = file_get_contents( $process_log );
	$data    = explode( PHP_EOL, $data );
	$details = json_decode( $data[0] );
	$details->completed_at = $now;
	$data[0] = json_encode( $details );
	file_put_contents( $process_log, implode( PHP_EOL, $data ) );
}