<?php

$targets = $args[0];

// Replaces dashes in keys with underscores
foreach($args as $index => $arg) {
	$split = strpos($arg, "=");
	if ( $split ) {
		$key = str_replace('-', '_', substr( $arg , 0, $split ) );
		$value = substr( $arg , $split, strlen( $arg ) );

		// Removes unnecessary bash quotes
		$value = trim( $value,'"' ); 				// Remove last quote 
		$value = str_replace( '="', '=', $value );  // Remove quote right after equals

		$args[$index] = $key.$value;
	} else {
		$args[$index] = str_replace('-', '_', $arg);
	}

}

// Converts --arguments into $arguments
parse_str( implode( '&', $args ), $arguments );

$arguments      = (object) $arguments;

if ( !isset( $targets ) ) {
	echo 'Error: Please specify a target @all, @production or @staging.';
	return;
}

// Process sites to target
$targets       = explode( ".", $targets );
$minor_targets = [];
if ( in_array( "@production", $targets ) ) {
	$environment = "Production";
}
if ( in_array( "@staging", $targets ) ) {
	$environment = "Staging";
}
if ( in_array( "@all", $targets ) ) {
	$environment = "all";
}
if ( in_array("monitor-on", $targets ) ) {
	$minor_targets[] = "monitor-on";
}
if ( in_array("updates-on", $targets ) ) {
	$minor_targets[] = "updates-on";
}
if ( in_array("updates-off", $targets ) ) {
	$minor_targets[] = "updates-off";
}
if ( in_array("offload-on", $targets ) ) {
	$minor_targets[] = "offload-on";
}
if ( in_array("offload-off", $targets ) ) {
	$minor_targets[] = "offload-off";
}
if ( ! empty( $filter ) && $filter != "core" && $filter != "plugins" && $filter != "themes" ) {
	echo 'Error: `--filter` can only be set to core, themes or plugins.';
	return;
}

$results        = [];
$site_arguments = [];
if ( ! empty( $arguments->filter ) ) {
	$site_arguments[ "filter"] = [
		"type"    => $arguments->filter,
		"name"    => empty( $arguments->filter_name ) ? "" : $arguments->filter_name,
		"version" => empty( $arguments->filter_version ) ? "" : $arguments->filter_version,
		"status"  => empty( $arguments->filter_status ) ? "" : $arguments->filter_status,
	];
}
if ( ! empty( $environment ) ) {
	$site_arguments[ "environment"] = $environment;
}
if ( ! empty( $arguments->provider ) ) {
	$site_arguments[ "provider"] = $arguments->provider;
}
if ( ! empty( $arguments->field ) ) {
	$site_arguments[ "field"] = $arguments->field;
}
if ( ! empty( $targets ) ) {
	$site_arguments[ "targets"] = $targets;
}
$sites = ( new CaptainCore\Sites )->fetch_sites_matching( $site_arguments );

if ( ! is_array( $sites ) ) {
	return;
}

foreach ( $sites as $site ) {
	$environment = strtolower ( $site->environment );
	$to_add      = "{$site->site}-{$environment}";
	if ( ! empty( $arguments->field ) ) {
		$to_add = $site->{$arguments->field};
	}
	if ( ! empty( $arguments->field ) && strpos( $arguments->field, ',' ) !== false ) {
		$fields = explode( ",", $arguments->field );
		$values = [];
		foreach ( $fields as $item ) {
			$values[] = $site->{$item};
		}
		$to_add = implode( ",", $values );
	}
	if ( empty( $to_add ) ) {
		continue;
	}
	$results[]   = $to_add;
}

// Return results
if ( ! empty( $arguments->field ) ) {
	if ( $arguments->field == 'ids' ) {
		$site = $site->site_id;
	} elseif ( $arguments->field == 'domain' ) {
		$site = $site->name;
	} else {
		$site = $site->{$arguments->field};
	}
	if ( isset( $debug ) ) {
		$site = "$site|DEBUG|{$site->name}";
	}
	if ( $site !=  "" ) {
		$results[] = $site;
	}
}

$results = array_unique( $results, SORT_REGULAR );
asort($results);
echo implode( ' ', $results );
