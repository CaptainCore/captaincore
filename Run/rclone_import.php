<?php
##
##		Preps new install for rclone via ~/.rclone.conf
##
## 		Pass arguments from command line like this
##		php rclone_import.php install=anchorhosting domain=anchor.host username=anchorhost password=random protocol=sftp port=2222
##

if (isset($argv)) {
	parse_str(implode('&', array_slice($argv, 1)), $_GET);
}

$install = $_GET['install'];
$address = $_GET['address'];
$username = $_GET['username'];
$password = $_GET['password'];
$protocol = $_GET['protocol'];
$port = $_GET['port'];

$file = $_SERVER['HOME'] . '/.rclone.conf';
if (!file_exists($file)) {
	// Try alternative location
	$file = $_SERVER['HOME'] . '/.config/rclone/rclone.conf';
}

$file = file_get_contents($file);

$pattern = '/\[(.+)\]\ntype\s=\ssftp\nhost\s=\s(.+)\nuser\s=\s(.+)\nport\s=\s(\d+)\npass\s=\s(.+)/';
preg_match_all($pattern, $file, $matches);

$found_install = false;
foreach ($matches[1] as $key => $value) {

  $prefix = 'sftp_';
  $value = substr($value, strlen($prefix));

  if ($value == $install) {
    $found_install = true;
  }

  #echo $matches[1][$key]."\n";
  #echo $matches[2][$key]."\n";
  #echo $matches[3][$key]."\n";

}

if ($found_install != true) {
  # Add to .rclone.conf file

	$lines = explode( PHP_EOL, $file);
	$line_count = count($lines);

	# Add new install to end of array
	$new_lines = array_slice($lines, 0, $line_count, true) +
	array("1n" => "[sftp-$install]") +
	array("2n" => "type = $protocol") +
	array("3n" => "host = $address") +
	array("4n" => "user = $username") +
	array("5n" => "port = $port") +
	array("6n" => "pass = $password") +
	array("7n" => "") +
	array_slice($lines, $key, count($lines) - 1, true);

	# outputs new additions to file
	$new_content = implode( PHP_EOL, $new_lines);
	file_put_contents($_SERVER['HOME'] . '/.rclone.conf', $new_content);

}
