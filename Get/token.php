<?php

//  Pass arguments from command line like this
//  php Scripts/Get/token.php domain=anchor.host
//  php Scripts/Get/token.php file=~/Sites/anchor.dev/wp-config.php

parse_str(implode('&', array_slice($argv, 1)), $_GET);

if (isset($_GET['domain'])) {
	$file = $_SERVER['HOME'] . "/Backup/" . $_GET['domain']. "/wp-config.php";

	if (file_exists($file)) {
		$wp_config = file_get_contents($file);

		preg_match('/define.*AUTH_KEY.*\'(.*)\'/', $wp_config, $value);
		$auth_key = $value[1];

		echo md5($auth_key);
	}
}

if (isset($_GET['file'])) {
	$file = $_GET['file'];

	if (file_exists($file)) {
		$wp_config = file_get_contents($file);

		preg_match('/define.*AUTH_KEY.*\'(.*)\'/', $wp_config, $value);
		$auth_key = $value[1];

		echo md5($auth_key);
	}
}
