<?php
$process_id = str_replace( "process-", "", basename( $argv[1]) );
$process_id = str_replace( "-progress.log", "", $process_id );
if ( $process_id == "running.json" || $process_id == "monitor.json" ) {
    return;
}
$format  = $argv[2];
$data    = file_get_contents ( $argv[1] );
$data    = explode( PHP_EOL, $data );
$details = json_decode( $data[0] );
$total   = (int) $details->count;
$current = isset( $data[1] ) ? strlen( $data[1] ) : 0;
$percent = $current / $total;

$percent_friendly = number_format( $percent * 100, 2 );

if ( $format == "json" ) {
    $response = [
        "command"    => $details->command,
        "created_at" => $details->created_at,
        "process_id" => $process_id,
        "percentage" => $percent_friendly,
        "status"     => "$current / $total completed",
    ];
    if ( ! empty ( $details->completed_at ) ) {
        $response["completed_at"] = $details->completed_at;
    }
    echo json_encode( $response ) . "\n";
    return;
}
echo "${percent_friendly}% - $current / $total completed\n";