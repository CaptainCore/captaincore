<?php 
echo "working";
$configurations = ( new CaptainCore\Configurations )->get();
print_r( $configurations  );
echo "working";
if ( $configurations["scheduled_tasks"] ) {
    print_r( $configurations["scheduled_tasks"] );
}