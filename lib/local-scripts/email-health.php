#! /usr/bin/env php
<?php
$command           = $argv[1];
$health_check_file = $argv[2];

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

// Check command: Feed in log file, outputs error count
if ( $command == 'check' ) {

    $email_checks = json_decode( file_get_contents( $health_check_file ) );
	foreach ( $email_checks as $email_check ) {
        if ( $email_check->status == "received" ) {
            continue;
        }
        $email_file = dirname( $health_check_file ) . "/response-{$email_check->site_id}-{$email_check->environment}.json";
        if ( is_file( $email_file ) ) {
            $email_check->status = "received";
        }
    }

    file_put_contents( $health_check_file, json_encode( $email_checks, JSON_PRETTY_PRINT ) );

}

// Process command: Feed in log file, outputs error urls and clean log file
if ( $command == 'process' ) {

	$lines  = explode( "\n", file_get_contents( $health_check_file ) );
	$output = [];
	$errors = [];

	foreach ( $lines as $line ) {

		$record = json_decode( $line );

		// Check if JSON valid
		if ( json_last_error() !== JSON_ERROR_NONE ) {
			continue;
		}

        $email_file = dirname( $health_check_file ) . "/response-{$record->site_id}-{$record->environment}.json";
        if ( is_file( $email_file ) ) {
            $record->status = "received";
        }

		$output[] = $record;

	}

	file_put_contents( $health_check_file, json_encode( $output, JSON_PRETTY_PRINT ) );

}

if ( $command == 'errors' ) {
    $email_checks = json_decode( file_get_contents( $health_check_file ) );
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

	$notify_at    = array( '1 hour', '4 hour', '24 hour' );
	$log_errors   = [];
	$time_now     = date( 'U' );
	$errors       = array();
	$known_errors = array();
	$warnings     = [];

	// Generate empty "/list.json" if needed
	if ( ! file_exists( $health_check_file ) ) {
		file_put_contents( $health_check_file, '[]' );
	}

	$email_checks = json_decode( file_get_contents( $health_check_file ) );

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

	}

	// If errors or restored then append ongoing errors to the bottom then output html
	if ( count( $errors ) > 0 ) {

		if ( count( $known_errors ) > 0 ) {
			$html .= '<br /><strong>Ongoing Errors</strong><br /><br />';
		}

		foreach ( $known_errors as $known_error ) {
			$html .= trim( $known_error ) . "<br />\n";
		};

		echo $html;

	}
}
