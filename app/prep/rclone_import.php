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

# rclone obscure password
$password = shell_exec('. '. $_SERVER['HOME'] . '/Scripts/config && $path_rclone/rclone obscure '. $password);

# locate rclone config file
$file_rclone_config = $_SERVER['HOME'] . '/.rclone.conf';
if (!file_exists($file_rclone_config)) {
	// Try alternative location
	$file_rclone_config = $_SERVER['HOME'] . '/.config/rclone/rclone.conf';
}

$file = file_get_contents($file_rclone_config);
$pattern = '/\[(.+)\]\ntype\s=\ssftp\nhost\s=\s(.+)\nuser\s=\s(.+)\nport\s=\s(\d+)\npass\s=\s/';
preg_match_all($pattern, $file, $matches);

$found_install = false;
foreach ($matches[1] as $key => $value) {

$prefix = 'sftp_';
$value = substr($value, strlen($prefix));

if ($value == $install) {
	$found_install = true;
}

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
array("6n" => "pass = $password");

# outputs new additions to file
$new_content = implode( PHP_EOL, $new_lines);
file_put_contents($file_rclone_config, $new_content);
echo "Added to rclone config\n";

} else {

# Update existing entry in .rclone.conf file

$lines = explode( PHP_EOL, $file);
$line_count = count($lines);

# Looks for duplicate install name
$seach_needle = "[sftp-$install]";
$key_search = array_search($seach_needle, $lines);

if ($key_search) {

	$i = 0;

	// finds last line of install
	do {
		if ($lines[$key_search + $i] == "") {
			$key_search_last = $key_search + $i;
		} $i++;
	} while ($lines[$key_search + $i -1] != "");

	// stored the number of lines removed
	$lines_removed = $i;

	// loop through and remove the current install
	for ($i = $key_search; $i <= $key_search_last; $i++) {
			unset($lines[$i]);
	}

	# Updates current install end of file
	$new_lines = array_slice($lines, 0, count($lines), true) +
	array("1n" => "[sftp-$install]") +
	array("2n" => "type = $protocol") +
	array("3n" => "host = $address") +
	array("4n" => "user = $username") +
	array("5n" => "port = $port") +
	array("6n" => "pass = $password");

	# outputs new additions to file
	$new_content = implode( PHP_EOL, $new_lines);
	file_put_contents($file_rclone_config, $new_content);

}

echo "Updating rclone config\n";
}
