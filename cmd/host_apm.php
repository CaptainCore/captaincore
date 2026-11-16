<?php
// Managed by CaptainCore (hosting configuration): APM. While the switch is on
// (until the time below), each page PHP builds is timed and written as one
// line to /var/lib/captaincore/apm/, which the server collects every minute.
// Nothing here shows in WordPress: no plugin, no admin screen. Commands
// (WP-CLI, WP-Cron) are not recorded.
//
// What a line holds: the transaction (what WordPress was asked for: a
// template, a REST route, an admin-ajax action), total time and memory, time
// in the database and in outside HTTP calls, and which plugin or theme
// caused them. Slow requests and a sample of the rest also get the phases of
// the page load, how long each plugin took to load, the slowest queries (SQL
// with every value replaced by ?) and every outside call (host and path,
// never the query string).

namespace CaptainCore\Apm;

const UNTIL      = 0; // replaced by host apply
const SLOW_MS    = 1000;
const DETAIL_PCT = 5;
const DIR        = '/var/lib/captaincore/apm';

if ( \PHP_SAPI === 'cli' || \PHP_SAPI === 'phpdbg' || \defined( 'CAPTAINCORE_APM' ) || \time() > UNTIL ) {
	return;
}
\define( 'CAPTAINCORE_APM', true );
// Query times come from WordPress's own query log. A site that sets
// SAVEQUERIES itself gets a "constant already defined" notice in its error
// log while APM is on, and keeps working.
if ( ! \defined( 'SAVEQUERIES' ) ) {
	\define( 'SAVEQUERIES', true );
}

final class R {
	public static $t0;
	public static $last;
	public static $marks  = [];
	public static $plugin = [];   // slug => [load ms, db ms, queries, http ms, http calls]
	public static $loads  = [];   // [slug, ms] in load order
	public static $open   = [];   // outside calls in flight: [url, start, comp, method]
	public static $calls  = [];   // [method, host, path, comp, ms, code]
	public static $tx     = '';
	public static $comps  = [];
	public static $done   = false;
	public static $dbf    = null;
	public static $q      = [];   // [sql, comp, ms]
}
R::$t0   = isset( $_SERVER['REQUEST_TIME_FLOAT'] ) ? (float) $_SERVER['REQUEST_TIME_FLOAT'] : \microtime( true );
R::$last = \microtime( true );

function ms( float $a, float $b ): float {
	return \round( ( $b - $a ) * 1000, 2 );
}

// Which plugin, mu-plugin or theme a file belongs to; 'core' for WordPress.
function comp( string $file ): string {
	if ( isset( R::$comps[ $file ] ) ) {
		return R::$comps[ $file ];
	}
	if ( \preg_match( '#/wp-content/(plugins|mu-plugins|themes)/([^/]+)#', $file, $m ) ) {
		$slug = \preg_replace( '/\.php$/', '', $m[2] );
		$c    = 'themes' === $m[1] ? 'theme:' . $slug : ( 'mu-plugins' === $m[1] ? 'mu:' . $slug : $slug );
	} elseif ( false !== \strpos( $file, '/etc/captaincore/' ) ) {
		$c = 'hosting';
	} else {
		$c = 'core';
	}
	return R::$comps[ $file ] = $c;
}

// Files of a database drop-in (Query Monitor's, HyperDB, LudicrousDB): a
// class that stands in for wpdb. Its own calls into wpdb are not the
// plugin that asked for the query.
function db_files(): array {
	if ( null !== R::$dbf ) {
		return R::$dbf;
	}
	R::$dbf = [];
	if ( isset( $GLOBALS['wpdb'] ) && \is_object( $GLOBALS['wpdb'] ) ) {
		for ( $c = new \ReflectionClass( $GLOBALS['wpdb'] ); $c && 'wpdb' !== $c->getName(); $c = $c->getParentClass() ) {
			R::$dbf[ (string) $c->getFileName() ] = true;
		}
	}
	return R::$dbf;
}

// The innermost plugin or theme on the call stack, else core.
function caller(): string {
	$skip = db_files();
	foreach ( \debug_backtrace( \DEBUG_BACKTRACE_IGNORE_ARGS, 80 ) as $f ) {
		if ( empty( $f['file'] ) || __FILE__ === $f['file'] || isset( $skip[ $f['file'] ] ) ) {
			continue;
		}
		$c = comp( $f['file'] );
		if ( 'core' !== $c ) {
			return $c;
		}
	}
	return 'core';
}

function add( string $comp, int $i, float $v ): void {
	if ( ! isset( R::$plugin[ $comp ] ) ) {
		R::$plugin[ $comp ] = [ 0.0, 0.0, 0, 0.0, 0 ];
	}
	R::$plugin[ $comp ][ $i ] += $v;
}

function mark( string $name ): void {
	R::$marks[] = [ $name, ms( R::$t0, \microtime( true ) ) ];
}

// The first mu-plugin has no hook before it (its time would include all of
// WordPress's start-up), so only the ones after it are timed.
function loaded_mu( $file ): void {
	if ( ! R::$loads ) {
		R::$last  = \microtime( true );
		R::$loads[] = [ comp( (string) $file ), null ];
		return;
	}
	loaded( $file );
}

function loaded( $file ): void {
	$now   = \microtime( true );
	$c     = comp( (string) $file );
	$took  = ms( R::$last, $now );
	R::$last = $now;
	add( $c, 0, $took );
	R::$loads[] = [ $c, $took ];
}

function hook( string $name, int $priority, callable $fn, int $args = 1 ): void {
	$GLOBALS['wp_filter'][ $name ][ $priority ][ 'captaincore_apm_' . $name ] = [ 'function' => $fn, 'accepted_args' => $args ];
}

// WordPress turns these preloaded arrays into hooks when it starts.
if ( ! isset( $GLOBALS['wp_filter'] ) || \is_array( $GLOBALS['wp_filter'] ) ) {
	hook( 'muplugins_loaded', \PHP_INT_MIN, function () { mark( 'mu-plugins' ); R::$last = \microtime( true ); }, 0 );
	hook( 'mu_plugin_loaded', \PHP_INT_MIN, __NAMESPACE__ . '\\loaded_mu' );
	hook( 'plugin_loaded', \PHP_INT_MIN, __NAMESPACE__ . '\\loaded' );
	hook( 'plugins_loaded', \PHP_INT_MIN, function () { mark( 'plugins' ); }, 0 );
	hook( 'after_setup_theme', \PHP_INT_MIN, function () { mark( 'theme' ); }, 0 );
	hook( 'init', \PHP_INT_MIN, function () { mark( 'init start' ); }, 0 );
	hook( 'wp_loaded', \PHP_INT_MIN, function () { mark( 'init' ); }, 0 );
	hook( 'wp', \PHP_INT_MIN, function () { mark( 'query' ); }, 0 );
	hook( 'template_redirect', \PHP_INT_MIN, function () { mark( 'redirects start' ); }, 0 );
	hook( 'template_include', \PHP_INT_MAX, function ( $t ) { mark( 'redirects' ); R::$tx = front(); return $t; } );
	// Every query, its time and the plugin or theme that asked for it, kept
	// here: WordPress's own query log can be emptied by others (Query Monitor
	// clears it for visitors it does not report to).
	hook( 'log_query_custom_data', \PHP_INT_MAX, function ( $data, $query, $time ) {
		R::$q[] = [ \substr( (string) $query, 0, 2000 ), caller(), (float) $time * 1000 ];
		return $data;
	}, 3 );
	hook( 'pre_http_request', \PHP_INT_MAX, function ( $pre, $args, $url ) {
		if ( false === $pre ) {
			R::$open[] = [ (string) $url, \microtime( true ), caller(), \strtoupper( (string) ( $args['method'] ?? 'GET' ) ) ];
		}
		return $pre;
	}, 3 );
	hook( 'http_api_debug', \PHP_INT_MAX, function ( $response, $context, $class, $args, $url ) {
		for ( $i = \count( R::$open ) - 1; $i >= 0; $i-- ) {
			if ( R::$open[ $i ][0] !== (string) $url ) {
				continue;
			}
			[ , $start, $c, $method ] = R::$open[ $i ];
			\array_splice( R::$open, $i, 1 );
			$took = ms( $start, \microtime( true ) );
			$p    = \parse_url( (string) $url );
			$code = \is_wp_error( $response ) ? 'error' : (int) ( $response['response']['code'] ?? 0 );
			R::$calls[] = [ $method, \strtolower( (string) ( $p['host'] ?? '' ) ), \substr( (string) ( $p['path'] ?? '/' ), 0, 80 ), $c, $took, $code ];
			add( $c, 3, $took );
			add( $c, 4, 1 );
			break;
		}
	}, 5 );
	hook( 'rest_dispatch_request', \PHP_INT_MAX, function ( $result, $request, $route ) {
		$route = \preg_replace( '/\(\?P<([a-zA-Z0-9_]+)>[^)]*\)/', '{$1}', (string) $route );
		R::$tx = 'REST ' . ( \is_object( $request ) && \method_exists( $request, 'get_method' ) ? $request->get_method() : '' ) . ' ' . \substr( $route, 0, 80 );
		return $result;
	}, 3 );
}

// What a front-end page is, in WordPress's terms.
function front(): string {
	if ( ! \function_exists( 'is_404' ) ) {
		return '';
	}
	if ( \function_exists( 'is_robots' ) && \is_robots() ) { return 'robots.txt'; }
	if ( \function_exists( 'is_favicon' ) && \is_favicon() ) { return 'favicon'; }
	if ( '' !== (string) \get_query_var( 'sitemap' ) ) { return 'sitemap'; }
	if ( \is_embed() ) { return 'embed'; }
	if ( \is_feed() ) { return 'feed'; }
	if ( \is_404() ) { return '404'; }
	if ( \is_search() ) { return 'search'; }
	if ( \is_front_page() ) { return 'front page'; }
	if ( \is_home() ) { return 'blog index'; }
	if ( \is_page() ) {
		$uri = \function_exists( 'get_page_uri' ) ? (string) \get_page_uri( \get_queried_object_id() ) : '';
		return 'page /' . \substr( $uri, 0, 60 ) . ( '' === $uri ? '' : '/' );
	}
	if ( \is_singular() ) { return 'single ' . \get_post_type(); }
	if ( \is_post_type_archive() ) { return 'archive ' . (string) \get_query_var( 'post_type' ); }
	if ( \is_category() ) { return 'category archive'; }
	if ( \is_tag() ) { return 'tag archive'; }
	if ( \is_tax() ) { $o = \get_queried_object(); return 'taxonomy ' . ( \is_object( $o ) ? $o->taxonomy : '' ); }
	if ( \is_author() ) { return 'author archive'; }
	if ( \is_date() ) { return 'date archive'; }
	return 'other';
}

function name(): string {
	$script = \basename( (string) ( $_SERVER['SCRIPT_NAME'] ?? '' ) );
	if ( \defined( 'DOING_AJAX' ) && \DOING_AJAX ) {
		$a = (string) ( $_REQUEST['action'] ?? '' );
		return 'admin-ajax: ' . ( \preg_match( '/^[A-Za-z0-9_\-.]{1,60}$/', $a ) ? $a : '?' );
	}
	if ( '' !== R::$tx ) {
		return R::$tx;
	}
	if ( \in_array( $script, [ 'wp-login.php', 'xmlrpc.php', 'wp-cron.php', 'wp-comments-post.php', 'wp-trackback.php' ], true ) ) {
		return $script;
	}
	if ( \function_exists( 'is_admin' ) && \is_admin() ) {
		$page = (string) ( $_GET['page'] ?? '' );
		return 'admin: ' . (string) ( $GLOBALS['pagenow'] ?? $script ) . ( \preg_match( '/^[A-Za-z0-9_\-.]{1,60}$/', $page ) ? '?page=' . $page : '' );
	}
	$code = (int) \http_response_code();
	if ( $code >= 300 && $code < 400 ) {
		return 'redirect';
	}
	if ( \function_exists( 'did_action' ) && \did_action( 'wp' ) ) {
		return front();
	}
	return $script ?: 'other';
}

// SQL with every value replaced by ?, so no customer data leaves the query.
function sql( string $q ): string {
	$q = \preg_replace( "/'(?:[^'\\\\]|\\\\.|'')*'/s", '?', $q );
	$q = \preg_replace( '/"(?:[^"\\\\]|\\\\.)*"/s', '?', $q );
	$q = \preg_replace( '/\b0x[0-9a-f]+\b|(?<![\w`.])-?\d+(?:\.\d+)?\b/i', '?', $q );
	$q = \preg_replace( '/\(\s*\?(?:\s*,\s*\?)+\s*\)/', '(?)', $q );
	$q = \preg_replace( '/\s+/', ' ', \trim( $q ) );
	return \substr( $q, 0, 400 );
}

function finish(): void {
	if ( R::$done ) {
		return;
	}
	R::$done = true;
	$end  = \microtime( true );
	$tot  = ms( R::$t0, $end );
	$code = (int) \http_response_code();
	$db   = 0.0;
	$qn   = 0;
	$qs   = [];
	foreach ( R::$q as $q ) {
		$db += $q[2];
		$qn++;
		add( $q[1], 1, $q[2] );
		add( $q[1], 2, 1 );
	}
	$qs = R::$q;
	$detail = $tot >= SLOW_MS || \mt_rand( 1, 100 ) <= DETAIL_PCT;
	\usort( $qs, function ( $a, $b ) { return $b[2] <=> $a[2]; } );
	$top = [];
	foreach ( \array_slice( $qs, 0, $detail ? 25 : 5 ) as $q ) {
		$top[] = [ sql( $q[0] ), $q[1], \round( $q[2], 2 ) ];
	}
	$http = 0.0;
	foreach ( R::$calls as $c ) {
		$http += $c[4];
	}
	$pl = [];
	foreach ( R::$plugin as $c => $v ) {
		$pl[ $c ] = [ \round( $v[0], 2 ), \round( $v[1], 2 ), $v[2], \round( $v[3], 2 ), $v[4] ];
	}
	$uri = (string) \strtok( (string) ( $_SERVER['REQUEST_URI'] ?? '/' ), '?' );
	$rec = [
		't'    => (int) R::$t0,
		'tx'   => \substr( name(), 0, 120 ),
		'm'    => \substr( (string) ( $_SERVER['REQUEST_METHOD'] ?? 'GET' ), 0, 8 ),
		'u'    => \substr( $uri, 0, 160 ),
		'st'   => $code,
		'ms'   => $tot,
		'db'   => \round( $db, 2 ),
		'q'    => $qn,
		'http' => \round( $http, 2 ),
		'hn'   => \count( R::$calls ),
		'mem'  => \round( \memory_get_peak_usage( true ) / 1048576, 1 ),
		'pl'   => $pl,
		'qs'   => $top,
		'hc'   => \array_slice( R::$calls, 0, 30 ),
	];
	if ( $detail ) {
		$marks   = R::$marks;
		$marks[] = [ 'render', $tot ];
		$rec['d'] = [ 'ph' => $marks, 'ld' => R::$loads ];
	}
	$file = DIR . '/' . \gmdate( 'YmdHi', (int) $end ) . '.jsonl';
	// A runaway (a flood of requests) stops recording rather than filling the disk.
	if ( @\filesize( $file ) > 50 * 1048576 ) {
		return;
	}
	$line = \json_encode( $rec, \JSON_UNESCAPED_SLASHES | \JSON_PARTIAL_OUTPUT_ON_ERROR | \JSON_INVALID_UTF8_SUBSTITUTE );
	if ( \is_string( $line ) ) {
		@\file_put_contents( $file, $line . "\n", \FILE_APPEND | \LOCK_EX );
	}
}

// Last in line: WordPress's own shutdown work (and the page cache's) runs
// first, so it counts toward the page's time.
\register_shutdown_function( function () {
	\register_shutdown_function( __NAMESPACE__ . '\\finish' );
} );
