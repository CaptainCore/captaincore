<?php
##
##		Loads new install configurations into logins.sh via command line
##
## 		Pass arguments from command line like this
##		php new.php install=anchorhosting domain=anchor.host username=anchorhost password=random address=10.10.10.10 protocol=sftp port=2222
##

if (isset($argv)) {
	parse_str(implode('&', array_slice($argv, 1)), $_GET);
}

$install = $_GET['install'];
$domain = $_GET['domain'];
$username = $_GET['username'];
$password = base64_decode(urldecode($_GET['password']));
$address = $_GET['address'];
$protocol = $_GET['protocol'];
$port = $_GET['port'];
$preloadusers = isset($_GET['preloadusers']) ? $_GET['preloadusers'] : '';   // List of customer ID to which have users to preload.
$homedir = isset($_GET['homedir']) ? $_GET['homedir'] : '';
$s3accesskey = isset($_GET['s3accesskey']) ? $_GET['s3accesskey'] : '';
$s3secretkey = isset($_GET['s3secretkey']) ? $_GET['s3secretkey'] : '';
$s3bucket = isset($_GET['s3bucket']) ? $_GET['s3bucket'] : '';
$s3path = isset($_GET['s3path']) ? $_GET['s3path'] : '';

if ($install) {

## logins.sh

	# Reads current backup logins
	$file = $_SERVER['HOME'] . '/Scripts/logins.sh';
	$current = file_get_contents($file);
	$lines = explode( PHP_EOL, $current);

	# Find end of websites array
	$key = array_search("		*)", $lines);

	# Looks for duplicate install name
	$seach_needle = "\t\t$install)";
	$key_search = array_search($seach_needle, $lines);

	if ($key_search) {

		$i = 0;

		// finds last line of install
		do {
			if ($lines[$key_search + $i] == "\t\t\t;;") {
				$key_search_last = $key_search + $i;
			} $i++;
		} while ($lines[$key_search + $i -1] != "\t\t\t;;");

		// stored the number of lines removed
		$lines_removed = $i;

		// loop through and remove the current install
		for ($i = $key_search; $i <= $key_search_last; $i++) {
		    unset($lines[$i]);
		}

		$key = array_search("		*)", $lines);

		# Add new install to end of array
		$new_lines = array_slice($lines, 0, $key - $lines_removed, true) +
		array("1n" => "		". $install.")") +
		array("2n" => "			### FTP info") +
		array("3n" => "			domain=$domain") +
		array("4n" => "			username=$username") +
		array("5n" => "			password='$password'") +
		array("6n" => "			ipAddress='$address'") +
		array("7n" => "			protocol='$protocol'") +
		array("8n" => "			port='$port'") +
		array("9n" => "			preloadusers='$preloadusers'") +
		array("10n" => "			homedir='$homedir'") +
		($s3accesskey != "" ? array("11n" => "			s3accesskey='$s3accesskey'"): array() ) +
		($s3secretkey != "" ? array("12n" => "			s3secretkey='$s3secretkey'"): array() ) +
		($s3bucket != "" ? array("13n" => "			s3bucket='$s3bucket'"): array() ) +
		($s3path != "" ? array("14n" => "			s3path='$s3path'"): array() ) +
		array("15n" => "			;;") +
		array_slice($lines, $key - $lines_removed, count($lines) - 1, true);


		if ($new_lines) {
			# outputs new additions to file
			$new_contents = implode( PHP_EOL, $new_lines);
			file_put_contents($_SERVER['HOME'] . '/Tmp/logins.sh', $new_contents);
		} else {
			# outputs new additions to file
			$new_contents = implode( PHP_EOL, $lines);
			file_put_contents($_SERVER['HOME'] . '/Tmp/logins.sh', $new_contents);
		}

	} else {

		# Duplicate key not found. Adds new install to the end of the file.

		# Add new install to end of array
		$new_lines = array_slice($lines, 0, $key, true) +
		array("1n" => "		". $install.")") +
		array("2n" => "			### FTP info") +
		array("3n" => "			domain=$domain") +
		array("4n" => "			username=$username") +
		array("5n" => "			password='$password'") +
		array("6n" => "			ipAddress='$address'") +
		array("7n" => "			protocol='$protocol'") +
		array("8n" => "			port='$port'") +
		array("9n" => "			preloadusers='$preloadusers'") +
		array("10n" => "			homedir='$homedir'") +
		($s3accesskey != "" ? array("11n" => "			s3accesskey='$s3accesskey'"): array() ) +
		($s3secretkey != "" ? array("12n" => "			s3secretkey='$s3secretkey'"): array() ) +
		($s3bucket != "" ? array("13n" => "			s3bucket='$s3bucket'"): array() ) +
		($s3path != "" ? array("14n" => "			s3path='$s3path'"): array() ) +
		array("15n" => "			;;") +
		array_slice($lines, $key, count($lines) - 1, true);

		# outputs new additions to file
		$new_contents = implode( PHP_EOL, $new_lines);
		file_put_contents($_SERVER['HOME'] . '/Tmp/logins.sh', $new_contents);
	}

	echo "Skipping plugins and backup\n";
	## 	setups up token and load custom configs into wp-config.php and .htaccess
	##  in a background process. Sent email when completed.
	$output = shell_exec($_SERVER['HOME'] . '/Scripts/Run/new_install_configs.sh '. $install .' > /dev/null 2>/dev/null &');

}

## Rclone Import

	# rclone obscure password
	$password = shell_exec('. '. $_SERVER['HOME'] . '/Scripts/config.sh && $path_rclone/rclone obscure '. $password);

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
	array("6n" => "pass = $password") +
	array("7n" => "");

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
		array("6n" => "pass = $password") +
		array("7n" => "");

		# outputs new additions to file
		$new_content = implode( PHP_EOL, $new_lines);
		file_put_contents($file_rclone_config, $new_content);

	}

	echo "Updating rclone config\n";
}

echo "Setting up ". $install;

?>
