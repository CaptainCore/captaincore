<?php
##
##		Extracts transferred stats from log files
##
## 		Pass arguments from command line like this
##		php calculate_transferred.php file=Logs/2017-07-01/20-25-f6c28f5/site-anchorhost-dropbox.txt
##

parse_str(implode('&', array_slice($argv, 1)), $_GET);

if ($_GET && $_GET['file']) {
	$file = $_GET['file'];
} else {
	$file = "~/Logs/anchor_dropbox_log_overall.txt";
}

if (file_exists($file)) {
$file = file_get_contents($file);
	// Bytes
	$pattern = '/(\d.*)(?= Bytes )/';
	preg_match_all($pattern, $file, $matches);
	$total_bytes = array_sum($matches[0]);

	// KBs
	$pattern = '/(\d.*)(?= kBytes )/';
	preg_match_all($pattern, $file, $matches);
	$total_kbytes = array_sum($matches[0]);

	// MBs
	$pattern = '/(\d.*)(?= MBytes )/';
	preg_match_all($pattern, $file, $matches);
	$total_mbytes = array_sum($matches[0]);

	// GBs
	$pattern = '/(\d.*)(?= GBytes )/';
	preg_match_all($pattern, $file, $matches);
	$total_gbytes = array_sum($matches[0]);

	// Add it all up
	$total_gb = round($total_bytes / 1024 / 1024 / 1024, 2) + round($total_kbytes / 1024 / 1024, 2) + round($total_mbytes / 1024, 2) + round($total_gbytes, 2);

	// Errors
	$pattern = '/(\d.*)(?=\sChecks)/';
	preg_match_all($pattern, $file, $matches);
	$total_errors = array_sum($matches[0]);

	// Checks
	$pattern = '/(\d.*)(?=\sTransferred)/';
	preg_match_all($pattern, $file, $matches);
	$total_checks = array_sum($matches[0]);

	// Transferred
	$pattern = '/(\d.*)(?=\sElapsed time)/';
	preg_match_all($pattern, $file, $matches);
	$total_transferred = array_sum($matches[0]);

	// return GBs transferred
	echo "Total data transferred: " . $total_gb ." GB<br>";
	echo "Total file errors: " . $total_errors . "<br>";
	echo "Total file checkes: " . $total_checks . "<br>";
	echo "Total files transferred: ". $total_transferred . "<br>";
}

?>
