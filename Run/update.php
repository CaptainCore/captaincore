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

$new_install = $_GET['install'];
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

if ($new_install) {

## logins.sh

	# Reads current backup logins
	$file = $_SERVER['HOME'] . '/Scripts/logins.sh';
	$current = file_get_contents($file);
	$lines = explode( PHP_EOL, $current);

	# Find end of websites array
	$key = array_search("		*)", $lines);

	# Looks for duplicate install name
	$seach_needle = "\t\t$new_install)";
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
		array("1n" => "		". $new_install.")") +
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
		array("1n" => "		". $new_install.")") +
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
	$output = shell_exec($_SERVER['HOME'] . '/Scripts/Run/new_install_configs.sh '. $new_install .' > /dev/null 2>/dev/null &');

}

echo "Setting up ". $new_install;

?>
