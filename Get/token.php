<?php

//  Pass arguments from command line like this
//  php get.php domain=anchor.host

parse_str(implode('&', array_slice($argv, 1)), $_GET);

$file = $_SERVER['HOME'] . "/Backup/" . $_GET['domain']. "/wp-config.php";

if (file_exists($file)) {
	$wp_config = file_get_contents($file);

	preg_match('/define.*AUTH_KEY.*\'(.*)\'/', $wp_config, $value);
	$auth_key = $value[1];

	echo md5($auth_key);
}

?>