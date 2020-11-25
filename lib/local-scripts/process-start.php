<?php
$process_id = $argv[1];
$count      = $argv[2];
$log        = $argv[3];
$running    = dirname( $log ) . "/running.json";
$processes  = json_decode( file_get_contents( $running ) );
foreach( $processes as $process ) {
    if ( $process->process_id == $process_id ) {
        $details = [
            "process_id" => $process->process_id,
            "command"    => $process->command,
            "created_at" => $process->created_at,
            "count"      => $count,
        ];
        file_put_contents( $log, json_encode( $details ) . "\n" );
    }
}