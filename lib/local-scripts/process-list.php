<?php
$running      = $argv[1];
$processes    = json_decode( file_get_contents( $running ) );
$process_ids  = array_column( $processes, "process_id" );
$path         = dirname( $running );
$process_logs = shell_exec( "ls ${path}/process-*-progress.log 2>/dev/null" );
$process_logs = explode( PHP_EOL, $process_logs );

foreach ( $process_logs as $process_log ) {
    if ( strpos($process_log, '-progress.log' ) === false ) { 
        continue;
    }
    $process = json_decode( shell_exec( "php {$_SERVER['HOME']}/.captaincore-cli/lib/local-scripts/process-track.php {$process_log} \"json\"" ) );
    if ( in_array( $process->process_id, $process_ids ) ) {
        foreach ( $processes as $p ) {
            if ( $p->process_id == $process->process_id ) {
                $p->status     = $process->status;
                $p->percentage = $process->percentage;
            }
        }
    } else {
        $processes[] = $process;
    }
}

echo json_encode( $processes, JSON_PRETTY_PRINT );