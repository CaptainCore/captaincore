<?php


$website_ids = array("18610","604");
$pattern = "^";
foreach ($website_ids as $website_id) {
	$pattern .= '(?=.*"'.$website_id.'")';
}
$pattern .= ".*$";

echo $pattern;

$arguments = array(
	'post_type'      => 'captcore_processlog',
	'posts_per_page' => '-1',
	'fields'         => 'ids',
	'meta_query'	=> array(
		array(
			'key'	 	=> 'website',
			'value'	  	=> $pattern,
			'compare' 	=> 'REGEXP',
		),
));

$process_logs = get_posts($arguments);

echo count( $process_logs );
