<?php
##
##		Returns server for given IP Address
##
## 		Pass arguments from command line like this
##		php Scripts/Get.server.php ip=104.198.197.214
##

parse_str(implode('&', array_slice($argv, 1)), $_GET);

$ip = $_GET["ip"];

$servers = array(
    "104.154.73.96" => "4972",
    "104.198.197.214" => "4970",
    "35.192.39.216" => "4954",
    "130.211.124.248" => "6283",
    "104.197.69.102" => "4967"
);

if (isset($servers[$ip])) {
  echo $servers[$ip];
}
