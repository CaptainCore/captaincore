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
$skip = $_GET['skip'];
$remove = $_GET['remove'];
$preloadusers = $_GET['preloadusers'];   // List of customer ID to which have users to preload.
$homedir = $_GET['homedir'];

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

		if ($remove != "true") {

			# Add new install to end of array
			$new_lines = array_slice($lines, 0, $key - $lines_removed, true) +
			array("1n" => "		". $new_install.")") +
			array("2n" => "			### FTP info") +
			array("3n" => "			echo \"Credentials loaded for install: \$website\"") +
			array("4n" => "			domain=$domain") +
			array("5n" => "			username=$username") +
			array("6n" => "			password='$password'") +
			array("7n" => "			ipAddress='$address'") +
			array("8n" => "			protocol='$protocol'") +
			array("9n" => "			port='$port'") +
			array("10n" => "			preloadusers='$preloadusers'") +
			array("11n" => "			homedir='$homedir'") +
			array("12n" => "			;;") +
			array_slice($lines, $key - $lines_removed, count($lines) - 1, true);

		}

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

		# Add new install to end of array
		$new_lines = array_slice($lines, 0, $key, true) +
		array("1n" => "		". $new_install.")") +
		array("2n" => "			### FTP info") +
		array("3n" => "			echo \"Credentials loaded for install: \$website\"") +
		array("4n" => "			domain=$domain") +
		array("5n" => "			username=$username") +
		array("6n" => "			password='$password'") +
		array("7n" => "			ipAddress='$address'") +
		array("8n" => "			protocol='$protocol'") +
		array("9n" => "			port='$port'") +
		array("10n" => "			preloadusers='$preloadusers'") +
		array("11n" => "			homedir='$homedir'") +
		array("12n" => "			;;") +
		array_slice($lines, $key, count($lines) - 1, true);

		# outputs new additions to file
		$new_contents = implode( PHP_EOL, $new_lines);
		file_put_contents($_SERVER['HOME'] . '/Tmp/logins.sh', $new_contents);
	}
	if ($skip == "true") {
		echo "Skipping plugins and backup\n";
		## 	setups up token and load custom configs into wp-config.php and .htaccess
		##  in a background process. Sent email when completed.
		$output = shell_exec($_SERVER['HOME'] . '/Scripts/Run/new_install_configs.sh '. $new_install .' > /dev/null 2>/dev/null &');

	} else {
		## 	run initial backup, setups up token, install plugins
		##	and load custom configs into wp-config.php and .htaccess
		##  in a background process. Sent email when completed.
		$output = shell_exec($_SERVER['HOME'] . '/Scripts/Run/new_install.sh '. $new_install .' > /dev/null 2>/dev/null &');
	}

	// Runs cleanup if install was removed. Also makes sure that the $domain contains at least a period.
	if ($remove == "true" and strpos($domain, '.') !== false) {
		$remove_directory = shell_exec('rm -rf ' . $_SERVER['HOME'] . '/Backup/'. $domain .' > /dev/null 2>/dev/null &');
	}

}

# echo $output;

echo "Setting up ". $new_install;

?>
