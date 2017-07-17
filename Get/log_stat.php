<?php
##
##		Extracts last group of transferred stats from log file
##
## 		Pass arguments from command line like this
##		php log_stats.php log=Logs/2017-07-01/20-25-f6c28f5/site-anchorhost-dropbox.txt
##
##		assign command line arguments to varibles
## 		new=anchorhosting becomes $_GET['new']
##

parse_str(implode('&', array_slice($argv, 1)), $_GET);

$file = $_GET['log'];

if (file_exists($file)) {
  $file = file_get_contents($file);

  // Match results groups
  $pattern = '/Transferred:(.+)\nErrors:(.+)\nChecks:(.+)\nTransferred:(.+)\nElapsed time:(.+)/';
  preg_match_all($pattern, $file, $matches);

}

// Last item in array
$match_count = count($matches[0]) - 1;

##    Finds last match. Example:
##
##    Transferred:   2.189 GBytes (166.310 kBytes/s)
##    Errors:                 0
##    Checks:             13958
##    Transferred:        13954
##    Elapsed time:   3h50m0.6s

$last_match = $matches[0][$match_count];

// Bytes
$pattern = '/(\d.*)(?= Bytes )/';
preg_match_all($pattern, $last_match, $matches);
$total_bytes = array_sum($matches[0]);

// KBs
$pattern = '/(\d.*)(?= kBytes )/';
preg_match_all($pattern, $last_match, $matches);
$total_kbytes = array_sum($matches[0]);

// MBs
$pattern = '/(\d.*)(?= MBytes )/';
preg_match_all($pattern, $last_match, $matches);
$total_mbytes = array_sum($matches[0]);

// GBs
$pattern = '/(\d.*)(?= GBytes )/';
preg_match_all($pattern, $last_match, $matches);
$total_gbytes = array_sum($matches[0]);

// Add it all up
$total_gb = round($total_bytes / 1024 / 1024 / 1024, 2) + round($total_kbytes / 1024 / 1024, 2) + round($total_mbytes / 1024, 2) + round($total_gbytes, 2);

// Errors
$pattern = '/(\d.*)(?=\sChecks)/';
preg_match_all($pattern, $last_match, $matches);
$total_errors = array_sum($matches[0]);

// Checks
$pattern = '/(\d.*)(?=\sTransferred)/';
preg_match_all($pattern, $last_match, $matches);
$total_checks = array_sum($matches[0]);

// Transferred
$pattern = '/(\d.*)(?=\sElapsed time)/';
preg_match_all($pattern, $last_match, $matches);
$total_transferred = array_sum($matches[0]);

// return GBs transferred
echo "Total data transferred: " . $total_gb ." GB<br>";
echo "Total file errors: " . $total_errors . "<br>";
echo "Total file checkes: " . $total_checks . "<br>";
echo "Total files transferred: ". $total_transferred . "<br>";
