<?php
##
##		Extrats stats from Restic output
##
## 		Pass arguments from command line like this
##		php restic_stats.php log=/Users/austinginder/Logs/2017-06-29/09-16-4a06d39/backup_b2.txt
##
##		assign command line arguments to varibles
## 		new=anchorhosting becomes $_GET['new']
##

parse_str(implode('&', array_slice($argv, 1)), $_GET);

$file = $_GET['log'];
$data_processed = 0;
$total_data_processed = 0;

if (file_exists($file)) {

  $file = file_get_contents($file);

  // Total data processed
  $pattern = '/(\[.+\] .+\/s\s+)(.+?)(\s\/\s)(.+?)(\s\d+)(\s\/\s)(\d+)/';
  preg_match_all($pattern, $file, $matches);

}

$data_processed_bytes = [];
$data_processed_total_bytes = [];

foreach ($matches[2] as $key => $value) {

  $data_processed = $value;
  $data_processed_total = $matches[4][$key];

  // Strips out Gib, MiB etc
  $data_processed_numeric = preg_replace("/[^0-9,.]/", "", $data_processed);
  $data_processed_total_numeric = preg_replace("/[^0-9,.]/", "", $data_processed_total);

  // Calculate bytes count
  if (strpos($data_processed, 'TiB') !== false) {
    $bytes = $data_processed_numeric * 1024 * 1024 * 1024 * 1024;
  }
  if (strpos($data_processed_total, 'TiB') !== false) {
    $bytes_total = $data_processed_total_numeric * 1024 * 1024 * 1024 * 1024;
  }

  if (strpos($data_processed, 'GiB') !== false) {
    $bytes = $data_processed_numeric * 1024 * 1024 * 1024;
  }
  if (strpos($data_processed_total, 'GiB') !== false) {
    $bytes_total = $data_processed_total_numeric * 1024 * 1024 * 1024;
  }

  if (strpos($data_processed, 'MiB') !== false) {
    $bytes = $data_processed_numeric * 1024 * 1024;
  }
  if (strpos($data_processed_total, 'MiB') !== false) {
    $bytes_total = $data_processed_total_numeric * 1024 * 1024;
  }

  if (strpos($data_processed, 'KiB') !== false) {
    $bytes = $data_processed_numeric * 1024;
  }
  if (strpos($data_processed_total, 'KiB') !== false) {
    $bytes_total = $data_processed_total_numeric * 1024;
  }

  $data_processed_bytes[] = $bytes;
  $data_processed_total_bytes[] = $bytes_total;
}

// Add it all up
$total_data_processed = round(array_sum($data_processed_bytes) / 1024 / 1024 / 1024, 2);
$total_data_processed_total = round(array_sum($data_processed_total_bytes) / 1024 / 1024 / 1024, 2);

// return GBs transferred
echo "Total data processed: " . $total_data_processed ." GB / " . $total_data_processed_total ." GB<br>";

?>
