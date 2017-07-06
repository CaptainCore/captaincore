<?php
##
##		Loads new install configurations into config.sh and logins.sh via command line
##
## 		Pass arguments from command line like this
##		php log_stats.php log=/Users/austinginder/Logs/2017-06-29/09-16-4a06d39
##
##		assign command line arguments to varibles
## 		new=anchorhosting becomes $_GET['new']
##

parse_str(implode('&', array_slice($argv, 1)), $_GET);

$file = $_GET['log'];

if (file_exists($file)) {
  $file = file_get_contents($dir . $file);

  // New Files
  $pattern = '/(?<=New: )(\d.*)/';
  preg_match_all($pattern, $file, $matches);
  $new_files = array_sum($matches[0]);
  $total_new_files = $total_new_files + $new_files;

  // Modified Files
  $pattern = '/(?<=Modified: )(\d.*)/';
  preg_match_all($pattern, $file, $matches);
  $modified_files = array_sum($matches[0]);
  $total_modified_files = $total_modified_files + $modified_files;

  // Bytes
  $pattern = '/(\d.*)(?= bytes transferred)/';
  preg_match_all($pattern, $file, $matches);
  $bytes_transferred = array_sum($matches[0]);
  $total_bytes = $total_bytes + $bytes_transferred;

}

// Add it all up
$total_gb = round($total_bytes / 1024 / 1024 / 1024, 2);

// return GBs transferred
echo "Total files transferred: " . $total_new_files . " new and " . $total_modified_files . " modified<br>";
echo "Total data transferred: " . $total_gb ." GB<br>";

?>
