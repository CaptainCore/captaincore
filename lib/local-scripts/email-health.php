#! /usr/bin/env php
<?php
$command                = $argv[1];
$health_check_directory = $argv[2];

function time_elapsed_string( $datetime, $full = false ) {
	$now  = new DateTime();
	$ago  = new DateTime( $datetime );
	$diff = $now->diff( $ago );

	$diff->w  = floor( $diff->d / 7 );
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
	foreach ( $string as $k => &$v ) {
		if ( $diff->$k ) {
			$v = $diff->$k . ' ' . $v . ( $diff->$k > 1 ? 's' : '' );
		} else {
			unset( $string[ $k ] );
		}
	}

	if ( ! $full ) {
		$string = array_slice( $string, 0, 1 );
	}
	return $string ? implode( ', ', $string ) . ' ago' : 'just now';
}

if ( $command == 'check' ) {

    $email_checks = json_decode( file_get_contents( "{$health_check_directory}list.json" ) );
	foreach ( $email_checks as $email_check ) {
        if ( $email_check->status == "received" ) {
            continue;
        }
        $email_file = "{$health_check_directory}response-{$email_check->site_id}-{$email_check->environment}.txt";
        if ( is_file( $email_file ) ) {
            $email_check->status      = "received";
			$email_check->received_at = filectime( $email_file );
        }
    }

    file_put_contents( "{$health_check_directory}list.json", json_encode( $email_checks, JSON_PRETTY_PRINT ) );

}

// Process command: Feed in log file, outputs error urls and clean log file
if ( $command == 'process' ) {

	if ( ! file_exists( "{$health_check_directory}list.json" ) ) {
		file_put_contents( "{$health_check_directory}list.json", '[]' );
	}

	$email_checks = json_decode( file_get_contents( "{$health_check_directory}list.json" ) );

	$lines  = explode( "\n", file_get_contents(  "{$health_check_directory}log.json" ) );
	$output = [];
	$errors = [];

	foreach ( $lines as $line ) {

		$record = json_decode( $line );

		// Check if JSON valid
		if ( json_last_error() !== JSON_ERROR_NONE ) {
			continue;
		}

        $email_file = "{$health_check_directory}response-{$record->site_id}-{$record->environment}.txt";
        if ( is_file( $email_file ) ) {
            $record->status      = "received";
			$record->received_at = filectime( $email_file );
        }

		$output[] = $record;

	}

	file_put_contents( "{$health_check_directory}list.json", json_encode( $output, JSON_PRETTY_PRINT ) );

}

if ( $command == 'undelivered' ) {
    $email_checks = json_decode( file_get_contents( "{$health_check_directory}list.json" ) );
    $undelivered  = 0;
	foreach ( $email_checks as $email_check ) {
        if ( $email_check->status == "sent" ) {
            $undelivered++;
        }
    }
    echo $undelivered;
}

// Generate command: Store errors in monitor.json and send email if needed
if ( $command == 'generate' ) {

	$errors       = [];
	$known_errors = [];
	$warnings     = [];

	// Generate empty "/list.json" if needed
	if ( ! file_exists( "{$health_check_directory}list.json" ) ) {
		file_put_contents( "{$health_check_directory}list.json", '[]' );
	}

	$email_checks = json_decode( file_get_contents( "{$health_check_directory}list.json" ) );

	// Loop through monitor records and update/remove to $errors[] as needed
	foreach ( $email_checks as $key => $record ) {

        if ( ! empty( $record->sending_unexpected_response ) ) {
            $warnings[] = "{$record->home_url} unexpected sending response {$record->sending_unexpected_response}\n";
        }

        if ( $record->status == "received" ) {
            continue;
        }

        if ( $record->status == "sent" ) {
            $record->status = "failed to receive";
        }

        $errors[] = "{$record->home_url} status {$record->status}\n";
	}

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

		echo $html;

	}

}
