<?php
##
##		Remove install configurations into config.sh and logins.sh via command line
##
## 		Pass arguments from command line like this
##		php remove.php install=anchorhosting domain=anchor.host
## 
##		assign command line arguments to varibles 
## 		install=anchorhosting becomes $_GET['install']
##

if (isset($argv)) {
	parse_str(implode('&', array_slice($argv, 1)), $_GET);
}

$install = $_GET['install'];
$domain = $_GET['domain'];

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

		# outputs new file
		$new_contents = implode( PHP_EOL, $lines);
		file_put_contents($_SERVER['HOME'] . '/Tmp/logins.sh', $new_contents);
	
	}
	
	// Runs cleanup if install was removed. Also makes sure that the $domain contains at least a period.
	if (strpos($domain, '.') !== false) {
		$output = shell_exec('sudo sh ' . $_SERVER['HOME'] . '/Scripts/remove_install.sh '. $domain .' > /dev/null 2>/dev/null &');
		#$remove_directory = shell_exec($command);
	}

}

echo "Removed ". $install;

?>