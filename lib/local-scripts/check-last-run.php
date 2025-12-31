<?php
// Usage: php check-last-run.php <json-file-path> <threshold-string>

if ( ! isset( $argv[1] ) || ! isset( $argv[2] ) ) {
    echo "false";
    exit;
}

$file      = $argv[1];
$threshold = strtolower( $argv[2] );

// Normalize time format (e.g. "24h" -> "24 hours")
if ( preg_match( '/^(\d+)h$/', $threshold, $matches ) ) {
    $threshold = $matches[1] . " hours";
} elseif ( preg_match( '/^(\d+)d$/', $threshold, $matches ) ) {
    $threshold = $matches[1] . " days";
} elseif ( preg_match( '/^(\d+)m$/', $threshold, $matches ) ) {
    $threshold = $matches[1] . " minutes";
}

if ( ! file_exists( $file ) ) {
    echo "false";
    exit;
}

// 1. Check File Modification Time (The "Last Checked" time)
$file_mtime = filemtime( $file );

// 2. Check JSON Content (The "Last Success" time)
$data = json_decode( file_get_contents( $file ) );
$last_entry_time = 0;

if ( ! empty( $data ) && is_array( $data ) ) {
    $timestamps = [];
    foreach ( $data as $item ) {
        if ( isset( $item->created_at ) ) {
            $timestamps[] = (int) $item->created_at;
        } elseif ( isset( $item->time ) ) {
            $timestamps[] = strtotime( $item->time );
        }
    }
    if ( ! empty( $timestamps ) ) {
        $last_entry_time = max( $timestamps );
    }
}

// Determine the most recent activity (either a check or a new entry)
$last_activity = max( $file_mtime, $last_entry_time );

// Calculate threshold time
$cutoff = strtotime( "-$threshold" );

// Safety check: if strtotime failed, don't skip anything
if ( $cutoff === false ) {
    echo "false";
    exit;
}

// If the last run is NEWER than the cutoff, we return "true" (meaning: yes, skip this site)
if ( $last_activity > $cutoff ) {
    echo "true";
} else {
    echo "false";
}