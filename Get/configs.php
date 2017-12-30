<?php
##
##		Loads custom configurations into wp-config.php and .htaccess
##
## 		Pass arguments from command line like this
##		php configs.php wpconfig=~/Documents/Scripts/wp-config.php htaccess=~/Documents/Scripts/.htaccess
##
##		assign command line arguments to varibles
## 		wpconfig=~/Documents/Scripts/wp-config.php becomes $_GET['wpconfig']
##

if (isset($argv)) {
	parse_str(implode('&', array_slice($argv, 1)), $_GET);
}

$wpconfig = $_GET['wpconfig'];
$htaccess = $_GET['htaccess'];
$domain = basename(dirname($wpconfig));

## customizes wp-config.php

if (file_exists($wpconfig)) {

	# Reads in wp-config.php file
	$file = $wpconfig;
	$current = file_get_contents($file);
	$lines = explode( PHP_EOL, $current);

	# Looks for a WordPress API Key
	$seach_needle = 'WPCOM_API_KEY';
	$key_search = array_find($seach_needle, $lines);

	if ($key_search) {
		echo "WPCOM_API_KEY already added\n";
	} else {

		# Find line number to begin adding custom configs
		$key = array_search('# WP Engine Settings', $lines);

		# If not found try a WordPress default
		if (!is_numeric($key)) {
			$key = array_search("/* That's all, stop editing! Happy blogging. */", $lines);
			if (is_numeric($key)) {
				// Found using WordPress default, output above the line
				$key = $key - 1;
			}
		}

		# Add new item to array at begin of array
		$new_lines = array_slice($lines, 0, $key + 1, true) +
		array("1n" => "define( 'WPCOM_API_KEY', '***REMOVED***' );") +
		array_slice($lines, $key, count($lines) - 1, true);

		# outputs new revisions to file
		$new_contents = implode( PHP_EOL, $new_lines);
		$output = file_put_contents($wpconfig, $new_contents);
	}

	# Reads in updated wp-config.php file
	$file = $wpconfig;
	$current = file_get_contents($file);
	$lines = explode( PHP_EOL, $current);

	# Looks for a Gravity Forms Key
	$seach_needle = 'GF_LICENSE_KEY';
	$key_search = array_find($seach_needle, $lines);

	if ($key_search) {
		echo "GF_LICENSE_KEY already added\n";
	} else {

		# Find line number to begin adding custom configs
		$key = array_search('# WP Engine Settings', $lines);

		# If not found try a WordPress default
		if (!is_numeric($key)) {
			$key = array_search("/* That's all, stop editing! Happy blogging. */", $lines);
			if (is_numeric($key)) {
				// Found using WordPress default, output above the line
				$key = $key - 1;
			}
		}

		# Add new item to array at begin of array
		$new_lines = array_slice($lines, 0, $key + 1, true) +
		array("1n" => "define( 'GF_LICENSE_KEY', '***REMOVED***' );") +
		array_slice($lines, $key, count($lines) - 1, true);

		# outputs new revisions to file
		$new_contents = implode( PHP_EOL, $new_lines);
		$output = file_put_contents($wpconfig, $new_contents);
	}

	# Reads in updated wp-config.php file
	$file = $wpconfig;
	$current = file_get_contents($file);
	$lines = explode( PHP_EOL, $current);

	# Looks for a Anchor Hosting Domain
	$seach_needle = 'ANCHORHOST_DOMAIN';
	$key_search = array_find($seach_needle, $lines);

	if ($key_search) {
		echo "ANCHORHOST_DOMAIN already added\n";
	} else {

		# Find line number to begin adding custom configs
		$key = array_search('# WP Engine Settings', $lines);

		# If not found try a WordPress default
		if (!is_numeric($key)) {
			$key = array_search("/* That's all, stop editing! Happy blogging. */", $lines);
			if (is_numeric($key)) {
				// Found using WordPress default, output above the line
				$key = $key - 1;
			}
		}

		# Add new item to array at begin of array
		$new_lines = array_slice($lines, 0, $key + 1, true) +
		array("1n" => "define( 'ANCHORHOST_DOMAIN', '". $domain."' );") +
		array_slice($lines, $key, count($lines) - 1, true);

		# outputs new revisions to file
		$new_contents = implode( PHP_EOL, $new_lines);
		$output = file_put_contents($wpconfig, $new_contents);
	}

	# Reads in updated wp-config.php file
	$file = $wpconfig;
	$current = file_get_contents($file);
	$lines = explode( PHP_EOL, $current);

	# Looks for ACF key
	$seach_needle = 'ACF_PRO_KEY';
	$key_search = array_find($seach_needle, $lines);

	if ($key_search) {
		echo "ACF_PRO_KEY already added\n";
	} else {

		# Find line number to begin adding custom configs
		$key = array_search('# WP Engine Settings', $lines);

		# If not found try a WordPress default
		if (!is_numeric($key)) {
			$key = array_search("/* That's all, stop editing! Happy blogging. */", $lines);
			if (is_numeric($key)) {
				// Found using WordPress default, output above the line
				$key = $key - 1;
			}
		}

		# Add new item to array at begin of array
		$new_lines = array_slice($lines, 0, $key + 1, true) +
		array("1n" => "define( 'ACF_PRO_KEY', '***REMOVED***' );") +
		array_slice($lines, $key, count($lines) - 1, true);

		# outputs new revisions to file
		$new_contents = implode( PHP_EOL, $new_lines);
		$output = file_put_contents($wpconfig, $new_contents);
	}
}

## customizes .htaccess
if (file_exists($htaccess)) {

	# Reads in .htaccess file
	$file = $htaccess;
	$current = file_get_contents($file);
	$lines = explode( PHP_EOL, $current);

	# Looks for post_max_size
	$seach_needle = "post_max_size";
	$key_search = array_find($seach_needle, $lines);

	if ($key_search) {
		echo "Already added php_value post_max_size\n";
	} else {

		# Find line number for websites array
		$key = array_search("# END WordPress", $lines);

		# Add new item to array at begin of array
		$new_lines = array_slice($lines, 0, $key + 1, true) +
		array("1n" => "php_value post_max_size 200M") +
		array_slice($lines, $key, count($lines) - 1, true);

		# outputs new revisions to file
		$new_contents = implode( PHP_EOL, $new_lines);
		file_put_contents($htaccess, $new_contents);
		echo "Added php_value post_max_size\n";
	}

	# Reads in .htaccess file
	$file = $htaccess;
	$current = file_get_contents($file);
	$lines = explode( PHP_EOL, $current);

	# Looks for post_max_size
	$seach_needle = "upload_max_filesize";
	$key_search = array_find($seach_needle, $lines);

	if ($key_search) {
		echo "Already added php_value upload_max_filesize\n";
	} else {

		# Find line number for websites array
		$key = array_search("# END WordPress", $lines);

		# Add new item to array at begin of array
		$new_lines = array_slice($lines, 0, $key + 1, true) +
		array("1n" => "php_value upload_max_filesize 200M") +
		array_slice($lines, $key, count($lines) - 1, true);

		# outputs new revisions to file
		$new_contents = implode( PHP_EOL, $new_lines);
		file_put_contents($htaccess, $new_contents);
		echo "Added php_value upload_max_filesize\n";
	}

}

/**
 * Case in-sensitive array_search() with partial matches
 *
 * @param string $needle   The string to search for.
 * @param array  $haystack The array to search in.
 *
 * @author Bran van der Meer <branmovic@gmail.com>
 * @since 29-01-2010
 */
function array_find($needle, array $haystack)
{
    foreach ($haystack as $key => $value) {
        if (false !== stripos($value, $needle)) {
            return $key;
        }
    }
    return false;
}

?>
