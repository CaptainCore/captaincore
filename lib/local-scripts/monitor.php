#! /usr/bin/env php
<?php

$command  = $argv[1];
$log_file = $argv[2];
if ( isset( $argv[3] ) ) {
	$urls_checked = $argv[3];
	$urls_checked = explode(" ", $urls_checked);
}

function time_elapsed_string($datetime, $full = false) {
    $now = new DateTime;
    $ago = new DateTime($datetime);
    $diff = $now->diff($ago);

    $diff->w = floor($diff->d / 7);
    $diff->d -= $diff->w * 7;

    $string = array(
        'y' => 'year',
        'm' => 'month',
        'w' => 'week',
        'd' => 'day',
        'h' => 'hour',
        'i' => 'minute',
        's' => 'second',
    );
    foreach ($string as $k => &$v) {
        if ($diff->$k) {
            $v = $diff->$k . ' ' . $v . ($diff->$k > 1 ? 's' : '');
        } else {
            unset($string[$k]);
        }
    }

    if (!$full) $string = array_slice($string, 0, 1);
    return $string ? implode(', ', $string) . ' ago' : 'just now';
}

function process_log( $action = "" ) {

	global $log_file;
	$contents = file_get_contents( $log_file );
	$lines    = explode( "\n", $contents );
	$output   = array();
	$errors   = array();

	foreach ( $lines as $line ) {

		$json = json_decode( $line );

		// Check if JSON valid
		if ( json_last_error() !== JSON_ERROR_NONE ) {
			continue;
		}

		$record = (object) [ 
			"http_code"  => $json->http_code,
			"url"        => $json->url,
			"html_valid" => $json->html_valid,
		];

		// Check if HTML is valid
		if ( $json->html_valid == 'false' ) {
			// Append to errors
			$errors[] = $record;
			continue;
		}

		// Check if healthy
		if ( $json->http_code == '200' ) {
			$output[] = $line;
			continue;
		}

		// Check for redirects
		if ( $json->http_code == '301' ) {
			$output[] = $line;
			continue;
		}

		// Append to errors
		$errors[] = $record;
	}

	if ( $action == "update" ) {

		// Update log file without errors
		$contents_updated = implode( "\n", $output );
		file_put_contents( $log_file, $contents_updated );

	}

	return $errors;
}

// Check command: Feed in log file, outputs error count
if ( $command == "check" ) {

	echo count( process_log() );

}

// Process command: Feed in log file, outputs error urls and clean log file
if ( $command == "process" ) {

	$errors = process_log( "update" );
	$urls = array_column( $errors, "url" );

	// Return URLs with errors
	echo implode( " ", $urls );

}

// Generate command: Store errors in monitor.json and send email if needed
if ( $command == "generate" ) {

	$notify_at    = array( "1 hour", "4 hour", "24 hour" );
	$monitor_json = dirname(__FILE__, 3) . "/data/monitor.json";
	$log_errors   = process_log();
	$time_now     = date( "U" );
	$errors       = array();
	$known_errors = array();
	$restored     = array();
	$warnings     = array();

	// Generate empty "/data/monitor.json" if needed
	if ( !file_exists( $monitor_json )) {
		file_put_contents( $monitor_json, "[]" );
	}

	$monitor_records = json_decode( file_get_contents( $monitor_json ) );

	// Store errors in monitor.json
	foreach( $log_errors as $log_error ) {

		$found = false;

		// See if url already in monitor.json
		foreach( $monitor_records as $record ) {

			// increase count
			if ( $record->url == $log_error->url ) {
				 $record->check_count = $record->check_count + 1;
				 $record->updated_at = $time_now;
				 $found = true;
				 break;
			}

		}

		if ( !$found ) {
			// Add to $monitor_records
			$monitor_records[] = (object) [
				"url"          => $log_error->url,
				"http_code"    => $log_error->http_code,
				"html_valid"   => $log_error->html_valid,
				"check_count"  => 1,
				"notify_count" => 0,
				"created_at"   => $time_now,
				"updated_at"   => $time_now,
			];

		}

	}

	// Loop through monitor records and update/remove to $errors[] as needed
	foreach( $monitor_records as $key => $record ) {

		// If existing monitor record not in original check, just remove it.
		if ( ! in_array( $record->url, $urls_checked ) and $record->notify_count != 0 ) {
			unset($monitor_records[$key]);
			continue;
		}
		
		// Check if online and remove from monitor.json
		if ( ! in_array( $record->url, array_column($log_errors, 'url') ) ) {
			$time_ago = date( "F j, Y, g:i a", $record->created_at );
			$restored[] = "{$record->url} has been restored. Was offline since $time_ago\n";
			unset($monitor_records[$key]);
			continue;
		}

		// Check if notifications count is exceeded, skip this record (Beyond 24hrs)
		if ( $record->notify_count >= count($notify_at) ) {
			$known_errors[] = "Response code {$record->http_code} on {$record->url} since $time_ago\n";
			continue;
		}

		if ( $record->notify_count == 0 ) {
			// If it's the first time then pass the time check
			$notify_time_check = $record->created_at;
		} else {
			// Otherwise calculate the time check based on $notify_count and $notify_at[]
			$key = $record->notify_count - 1;
			$notify_time_check = strtotime("-". $notify_at[$key]);
		}

		// Check if "notify at" time is ready, otherwise skip this record
		if ( $record->created_at > $notify_time_check ) {
			$known_errors[] = "Response code {$record->http_code} on {$record->url} since $time_ago\n";
			continue;
		}

		// Check if HTML is valid
		if ( $record->html_valid == 'false' ) {
			$record->notify_count = $record->notify_count + 1;
			$time_ago = time_elapsed_string('@'. $record->created_at);
			$errors[] = "Response code {$record->http_code} on {$record->url} html is invalid since $time_ago\n";
			continue;
		}

		// Check for redirects
		if ( $record->http_code == '301' ) {
			$warnings[] = "Response code {$record->http_code} on {$record->url}\n";
			continue;
		}

		// Append error to errors for email purposes
		$time_ago = time_elapsed_string('@'. $record->created_at);
		$errors[] = "Response code {$record->http_code} on {$record->url} since $time_ago\n";
		$record->notify_count = $record->notify_count + 1;

	}

	// Normalize your integer keys 
	$monitor_records = array_values($monitor_records);

	// Update monitor.json
	file_put_contents( $monitor_json, json_encode( $monitor_records, JSON_PRETTY_PRINT ) );

	// If errors then generate html
	if ( count( $errors ) > 0 ) {

		$html = '<strong>Errors</strong><br /><br />';

		foreach ( $errors as $error ) {
			$html .= trim( $error ) . "<br />\n";
		};
		
		if ( count( $warnings ) > 0 ) {
			$html .= '<br /><strong>Warnings</strong><br /><br />';
		}

		foreach ( $warnings as $warning ) {
			$html .= trim( $warning ) . "<br />\n";
		};

	}

	// If restored then generate html
	if ( count( $restored ) > 0 ) {

		$html = '<strong>Restored</strong><br /><br />';

		foreach ( $restored as $restore ) {
			$html .= trim( $restore ) . "<br />\n";
		};

	}

	// If errors or restored then append ongoing errors to the bottom then output html
	if ( count( $errors ) > 0 || count( $restored ) > 0 ) {
		
		if ( count( $known_errors ) > 0 ) {
			$html .= '<br /><strong>Ongoing Errors</strong><br /><br />';
		}

		foreach ( $known_errors as $known_error ) {
			$html .= trim( $known_error ) . "<br />\n";
		};

		echo $html;

	}

}