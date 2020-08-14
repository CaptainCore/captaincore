<?php

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
parse_str( implode( '&', $args ) );

if ( !isset( $targets ) ) {
	echo 'Error: Please specify a target @all, @production or @staging.';
	return;
}

// Process sites to target
$targets       = explode( ".", $targets );
$minor_targets = [];
if ( in_array( "production", $targets ) ) {
	$environment = "Production";
}
if ( in_array( "staging", $targets ) ) {
	$environment = "Staging";
}
if ( in_array( "all", $targets ) ) {
	$environment = "all";
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

$results  = [];
$sites = ( new CaptainCore\Sites )->fetch_sites_matching( [ 
	"filter" => [
		"type"    => $filter,
		"name"    => $filter_name,
		"version" => $filter_version,
		"status"  => $filter_status,
	],
	"environment" => $environment,
	"provider"    => $provider,
	"field"       => $field,
	"targets"     => $minor_targets,
] );

if ( ! is_array( $sites ) ) {
	return;
}

foreach ( $sites as $site ) {
	$environment = strtolower ( $site->environment );
	$to_add      = "{$site->site}-{$environment}";
	if ( ! empty( $field ) ) {
		$to_add = $site->{$field};
	}
	if ( ! empty( $field ) && strpos( $field, ',' ) !== false ) {
		$fields = explode( ",", $field );
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
if ( $field ) {
	if ( $field == 'ids' ) {
		$site = $site->site_id;
	} elseif ( $field == 'domain' ) {
		$site = $site->name;
	} else {
		$site = $site->{$field};
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
