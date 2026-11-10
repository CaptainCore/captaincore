<?php
/**
 * CaptainCore page cache. Managed by the hosting configuration: changes here
 * are overwritten by `captaincore host apply`.
 *
 * PHP loads this file before every request (auto_prepend_file), so the site
 * needs no caching plugin and nothing shows in WordPress. It does two jobs:
 *
 *  1. Saves the HTML of pages that anonymous visitors request, as static files
 *     under ROOT/<host>/<path>/. The web server in front of PHP serves those
 *     files itself on later requests; PHP is not involved in a cache hit.
 *  2. Deletes saved pages when content changes (WordPress hooks).
 *
 * Nothing here may ever break a request: every entry point swallows errors.
 */

namespace CaptainCore\PageCache;

if ( \defined( __NAMESPACE__ . '\\ROOT' ) ) {
	return;
}

const ROOT   = '/var/cache/captaincore/page';
const CONFIG = '/etc/captaincore/page-cache.json';

// Kept in step with the web server's rules (cmd/host_apply.py): a request
// carrying one of these cookies is never answered from the cache.
const SKIP_COOKIES = [ 'wordpress_logged_in_', 'wp-postpass_', 'comment_author_', 'woocommerce_items_in_cart', 'woocommerce_cart_hash', 'wp_woocommerce_session_', 'edd_items_in_cart', 'wordpress_no_cache' ];
// Query parameters that only track a visit: the page is the same with or without them.
const IGNORED_QUERY = '/^(utm_[a-z0-9_]+|fbclid|gclid|gclsrc|gad_source|gad_campaignid|gbraid|wbraid|dclid|msclkid|mc_cid|mc_eid|_ga|_gl|srsltid|ttclid|twclid|li_fat_id|igshid|yclid|_hsenc|_hsmi|mkt_tok)$/';
// Always dynamic, whatever the site's own list says.
const SKIP_PATHS = [ '/wp-admin', '/wp-json', '/wp-includes', '/wc-api', '/cart', '/checkout', '/my-account', '/search' ];
// A page that sends one of these itself is left uncached: a saved copy is a
// plain file and could not send the header again.
const KEEP_HEADERS = [ 'content-security-policy', 'content-security-policy-report-only', 'x-frame-options', 'permissions-policy', 'referrer-policy', 'strict-transport-security', 'cross-origin-opener-policy', 'cross-origin-embedder-policy', 'cross-origin-resource-policy', 'www-authenticate', 'location', 'content-encoding', 'content-disposition' ];
// Post types with no page of their own that still change every page.
const SITEWIDE_TYPES = [ 'wp_template', 'wp_template_part', 'wp_global_styles', 'wp_navigation', 'wp_block', 'nav_menu_item', 'custom_css', 'elementor_library' ];
const SITEWIDE_OPTIONS = [ 'sidebars_widgets', 'permalink_structure', 'blogname', 'blogdescription', 'show_on_front', 'page_on_front', 'page_for_posts', 'posts_per_page', 'WPLANG', 'site_icon', 'date_format', 'time_format', 'category_base', 'tag_base' ];
const MAX_BYTES = 4194304;

try {
	boot();
} catch ( \Throwable $e ) {
	// A cache that cannot start is a cache miss, never an error page.
}

function config(): array {
	static $config = null;
	if ( $config === null ) {
		$raw    = @file_get_contents( CONFIG );
		$config = \is_string( $raw ) ? ( json_decode( $raw, true ) ?: [] ) : [];
	}
	return $config;
}

function state( ?array $set = null ): array {
	static $state = [ 'candidate' => false, 'file' => '', 'full' => false, 'paths' => [], 'home' => false, 'old' => [], 'flushed' => false ];
	if ( $set !== null ) {
		$state = $set + $state;
	}
	return $state;
}

function boot(): void {
	$config = config();
	if ( empty( $config['enabled'] ) ) {
		return;
	}
	// WordPress builds its hooks from this array when it loads, so content
	// changes clear saved pages from the web, WP-CLI and cron alike.
	if ( ! isset( $GLOBALS['wp_filter'] ) || \is_array( $GLOBALS['wp_filter'] ) ) {
		$GLOBALS['wp_filter']['muplugins_loaded'][0]['captaincore_page_cache'] = [ 'function' => __NAMESPACE__ . '\\register', 'accepted_args' => 0 ];
	}
	if ( \PHP_SAPI === 'cli' || \PHP_SAPI === 'phpdbg' ) {
		return;
	}
	$file = target( $config );
	if ( $file === '' ) {
		@header( 'X-Cache: BYPASS' );
		return;
	}
	state( [ 'candidate' => true, 'file' => $file ] );
	@header( 'X-Cache: MISS' );
	ob_start( __NAMESPACE__ . '\\capture' );
}

/** The file this request's page would be saved to, or '' when it must stay dynamic. */
function target( array $config ): string {
	if ( ( $_SERVER['REQUEST_METHOD'] ?? '' ) !== 'GET' ) {
		return '';
	}
	if ( strtolower( (string) ( $_SERVER['HTTP_X_FORWARDED_PROTO'] ?? '' ) ) !== 'https' ) {
		return '';
	}
	if ( ! empty( $_SERVER['HTTP_AUTHORIZATION'] ) || ! empty( $_SERVER['REDIRECT_HTTP_AUTHORIZATION'] ) ) {
		return '';
	}
	$host = strtolower( (string) ( $_SERVER['HTTP_HOST'] ?? '' ) );
	if ( $host === '' || ! \in_array( $host, (array) ( $config['hosts'] ?? [] ), true ) ) {
		return '';
	}
	$uri   = (string) ( $_SERVER['REQUEST_URI'] ?? '' );
	$cut   = strpos( $uri, '?' );
	$path  = $cut === false ? $uri : substr( $uri, 0, $cut );
	$query = $cut === false ? '' : substr( $uri, $cut + 1 );
	if ( ! cacheable_path( $path, (array) ( $config['exclude'] ?? [] ) ) ) {
		return '';
	}
	if ( $query !== '' ) {
		foreach ( explode( '&', $query ) as $pair ) {
			$name = explode( '=', $pair, 2 )[0];
			if ( ! preg_match( IGNORED_QUERY, $name ) ) {
				return '';
			}
		}
	}
	foreach ( array_keys( $_COOKIE ) as $name ) {
		foreach ( SKIP_COOKIES as $prefix ) {
			if ( strncmp( (string) $name, $prefix, \strlen( $prefix ) ) === 0 ) {
				return '';
			}
		}
	}
	return page_file( $host, $path );
}

/** Plain ASCII page paths only: /, /about/, /2026/11/a-post (never files, never odd encodings). */
function cacheable_path( string $path, array $exclude ): bool {
	if ( $path === '' || $path[0] !== '/' || \strlen( $path ) > 240 || ! preg_match( '#^[A-Za-z0-9/_.~-]+$#', $path ) ) {
		return false;
	}
	if ( strpos( $path, '//' ) !== false || strpos( $path, '..' ) !== false || substr_count( $path, '/' ) > 12 ) {
		return false;
	}
	$trimmed = rtrim( $path, '/' );
	$last    = (string) substr( $trimmed, (int) strrpos( $trimmed, '/' ) + 1 );
	if ( strpos( $last, '.' ) !== false || $last === 'feed' ) {
		return false;
	}
	foreach ( array_merge( SKIP_PATHS, $exclude ) as $prefix ) {
		$prefix = rtrim( (string) $prefix, '/' );
		if ( $prefix !== '' && ( $trimmed === $prefix || strncmp( $trimmed . '/', $prefix . '/', \strlen( $prefix ) + 1 ) === 0 ) ) {
			return false;
		}
	}
	return true;
}

/** /about/ and /about are different URLs to WordPress, so each gets its own file. */
function page_file( string $host, string $path ): string {
	$name = substr( $path, -1 ) === '/' ? 'index.html' : 'index-noslash.html';
	$dir  = trim( $path, '/' );
	return ROOT . '/' . $host . ( $dir === '' ? '' : '/' . $dir ) . '/' . $name;
}

/** Output handler: passes the page through untouched and saves it when it is safe to reuse. */
function capture( string $buffer, int $phase ): string {
	static $partial = false;
	try {
		$whole = \PHP_OUTPUT_HANDLER_START | \PHP_OUTPUT_HANDLER_FINAL;
		if ( $partial || ( $phase & $whole ) !== $whole || ( $phase & \PHP_OUTPUT_HANDLER_CLEAN ) ) {
			$partial = true;   // flushed in pieces or thrown away: not a page we can save
			return $buffer;
		}
		if ( ! storable( $buffer ) ) {
			if ( ! headers_sent() ) {
				@header( 'X-Cache: BYPASS' );
			}
			return $buffer;
		}
		$file = state()['file'];
		$dir  = \dirname( $file );
		if ( ! is_dir( $dir ) && ! @mkdir( $dir, 0750, true ) && ! is_dir( $dir ) ) {
			return $buffer;
		}
		$tmp = $dir . '/.tmp-' . getmypid() . '-' . bin2hex( random_bytes( 4 ) );
		if ( @file_put_contents( $tmp, $buffer ) === \strlen( $buffer ) ) {
			@rename( $tmp, $file );
		} else {
			@unlink( $tmp );
		}
	} catch ( \Throwable $e ) {
		// The visitor still gets the page.
	}
	return $buffer;
}

function storable( string $buffer ): bool {
	$length = \strlen( $buffer );
	if ( $length < 512 || $length > MAX_BYTES || http_response_code() !== 200 || stripos( $buffer, '</html>' ) === false ) {
		return false;
	}
	if ( \defined( 'DONOTCACHEPAGE' ) && \constant( 'DONOTCACHEPAGE' ) ) {
		return false;
	}
	foreach ( headers_list() as $line ) {
		$parts = explode( ':', $line, 2 );
		$name  = strtolower( trim( $parts[0] ) );
		$value = strtolower( trim( $parts[1] ?? '' ) );
		if ( $name === 'set-cookie' || \in_array( $name, KEEP_HEADERS, true ) ) {
			return false;
		}
		if ( $name === 'content-type' && strncmp( $value, 'text/html', 9 ) !== 0 ) {
			return false;
		}
		if ( $name === 'cache-control' && preg_match( '/no-store|no-cache|private/', $value ) ) {
			return false;
		}
		if ( $name === 'vary' && preg_match( '/cookie|user-agent|\*/', $value ) ) {
			return false;
		}
	}
	// WordPress's own view of the request, when it got far enough to have one.
	if ( \function_exists( 'is_user_logged_in' ) && \function_exists( 'did_action' ) && did_action( 'init' ) && is_user_logged_in() ) {
		return false;
	}
	if ( isset( $GLOBALS['wp_query'] ) && \function_exists( 'did_action' ) && did_action( 'wp' ) ) {
		if ( is_404() || is_search() || is_feed() || is_preview() || is_trackback() ) {
			return false;
		}
		// Plain permalinks (?p=123): WordPress ignores the path and answers
		// the home page at any address, so only the home page is a page.
		$path = (string) strtok( (string) ( $_SERVER['REQUEST_URI'] ?? '' ), '?' );
		if ( $path !== '/' && \function_exists( 'get_option' ) && (string) get_option( 'permalink_structure' ) === '' ) {
			return false;
		}
		if ( is_singular() && \function_exists( 'post_password_required' ) && post_password_required() ) {
			return false;
		}
	}
	$free = @disk_free_space( ROOT );
	return $free === false || $free > 268435456;
}

// ── Clearing saved pages when content changes ───────────────────────────────

function register(): void {
	try {
		if ( ! \function_exists( 'add_action' ) ) {
			return;
		}
		$ns = __NAMESPACE__ . '\\';
		add_action( 'pre_post_update', $ns . 'on_before_update', 10, 1 );
		add_action( 'transition_post_status', $ns . 'on_post_status', 10, 3 );
		add_action( 'before_delete_post', $ns . 'on_post_changed', 10, 1 );
		add_action( 'clean_post_cache', $ns . 'on_clean_post', 10, 2 );
		add_action( 'edit_comment', $ns . 'on_comment', 10, 1 );
		add_action( 'woocommerce_product_set_stock', $ns . 'on_product', 10, 1 );
		add_action( 'woocommerce_variation_set_stock', $ns . 'on_product', 10, 1 );
		add_action( 'profile_update', $ns . 'on_user', 10, 1 );
		add_action( 'updated_option', $ns . 'on_option', 10, 1 );
		foreach ( [ 'switch_theme', 'customize_save_after', 'wp_update_nav_menu', 'activated_plugin', 'deactivated_plugin', 'upgrader_process_complete', 'edited_term', 'delete_term', 'deleted_user' ] as $hook ) {
			add_action( $hook, $ns . 'everything', 10, 0 );
		}
		add_action( 'shutdown', $ns . 'flush', 9999, 0 );
		register_shutdown_function( $ns . 'flush' );
	} catch ( \Throwable $e ) {
	}
}

function everything(): void {
	state( [ 'full' => true ] );
}

function viewable( $post ): bool {
	return \is_object( $post ) && ! empty( $post->post_type ) && \function_exists( 'is_post_type_viewable' ) && is_post_type_viewable( $post->post_type );
}

/** A published post's address before an update, in case the update moves it. */
function on_before_update( $post_id ): void {
	try {
		$post = get_post( $post_id );
		if ( viewable( $post ) && $post->post_status === 'publish' ) {
			$state           = state();
			$state['old'][]  = (string) get_permalink( $post );
			state( $state );
		}
	} catch ( \Throwable $e ) {
	}
}

function on_post_status( $new, $old, $post ): void {
	try {
		if ( $new !== 'publish' && $old !== 'publish' ) {
			return;
		}
		if ( \is_object( $post ) && \in_array( $post->post_type, SITEWIDE_TYPES, true ) ) {
			everything();
			return;
		}
		queue_post( $post, true );
	} catch ( \Throwable $e ) {
	}
}

function on_post_changed( $post_id ): void {
	try {
		$post = get_post( $post_id );
		if ( \is_object( $post ) && $post->post_status === 'publish' ) {
			queue_post( $post, true );
		}
	} catch ( \Throwable $e ) {
	}
}

/**
 * WordPress drops its own cached copy of a post (a comment, a stock level, a
 * field saved by a plugin): clear that page. Ignored while an anonymous page
 * view is being rendered, so a plugin that does this on every view cannot
 * keep its own page out of the cache.
 */
function on_clean_post( $post_id, $post = null ): void {
	try {
		if ( state()['candidate'] ) {
			return;
		}
		$post = \is_object( $post ) ? $post : get_post( $post_id );
		if ( \is_object( $post ) && $post->post_status === 'publish' ) {
			queue_post( $post, false );
		}
	} catch ( \Throwable $e ) {
	}
}

function on_comment( $comment_id ): void {
	try {
		$comment = get_comment( $comment_id );
		if ( $comment && ! empty( $comment->comment_post_ID ) ) {
			queue_post( get_post( (int) $comment->comment_post_ID ), false );
		}
	} catch ( \Throwable $e ) {
	}
}

function on_product( $product ): void {
	try {
		if ( \is_object( $product ) && method_exists( $product, 'get_id' ) ) {
			$id = method_exists( $product, 'get_parent_id' ) && $product->get_parent_id() ? $product->get_parent_id() : $product->get_id();
			queue_post( get_post( (int) $id ), false );
		}
	} catch ( \Throwable $e ) {
	}
}

function on_user( $user_id ): void {
	try {
		queue_url( (string) get_author_posts_url( (int) $user_id ) );
	} catch ( \Throwable $e ) {
	}
}

function on_option( $option ): void {
	if ( \in_array( $option, SITEWIDE_OPTIONS, true ) || strncmp( (string) $option, 'widget_', 7 ) === 0 || strncmp( (string) $option, 'theme_mods_', 11 ) === 0 ) {
		everything();
	}
}

/** The post's own page; with $listings also every page that lists it. */
function queue_post( $post, bool $listings ): void {
	if ( ! viewable( $post ) || wp_is_post_revision( $post ) || wp_is_post_autosave( $post ) ) {
		return;
	}
	queue_url( (string) get_permalink( $post ) );
	if ( ! $listings ) {
		return;
	}
	state( [ 'home' => true ] );
	$posts_page = (int) get_option( 'page_for_posts' );
	if ( $posts_page ) {
		queue_url( (string) get_permalink( $posts_page ) );
	}
	$archive = get_post_type_archive_link( $post->post_type );
	if ( $archive ) {
		queue_url( (string) $archive );
	}
	foreach ( get_object_taxonomies( $post->post_type, 'objects' ) as $taxonomy ) {
		if ( empty( $taxonomy->public ) ) {
			continue;
		}
		$terms = get_the_terms( $post, $taxonomy->name );
		foreach ( \is_array( $terms ) ? $terms : [] as $term ) {
			$link = get_term_link( $term );
			if ( \is_string( $link ) ) {
				queue_url( $link );
			}
		}
	}
	if ( ! empty( $post->post_author ) ) {
		queue_url( (string) get_author_posts_url( (int) $post->post_author ) );
	}
	$time = strtotime( (string) $post->post_date );
	if ( $time ) {
		queue_url( (string) get_year_link( (int) gmdate( 'Y', $time ) ) );
	}
}

function queue_url( string $url ): void {
	$path = (string) parse_url( $url, PHP_URL_PATH );
	if ( $path === '' || $path === '/' ) {
		state( [ 'home' => true ] );
		return;
	}
	if ( preg_match( '#^/[A-Za-z0-9/_.~-]+$#', $path ) && strpos( $path, '..' ) === false && strpos( $path, '//' ) === false ) {
		$state                                = state();
		$state['paths'][ trim( $path, '/' ) ] = true;
		state( $state );
	}
}

/** Runs once at the end of the request: one pass however many posts changed. */
function flush(): void {
	try {
		$state = state();
		if ( $state['flushed'] ) {
			return;
		}
		state( [ 'flushed' => true ] );
		foreach ( $state['old'] as $url ) {
			queue_url( $url );
		}
		$state = state();
		if ( ! $state['full'] && ! $state['home'] && ! $state['paths'] ) {
			return;
		}
		foreach ( (array) @scandir( ROOT ) as $host ) {
			if ( $host === '' || $host[0] === '.' || ! is_dir( ROOT . '/' . $host ) ) {
				continue;
			}
			$base = ROOT . '/' . $host;
			if ( $state['full'] ) {
				// Out of the web server's sight in one step, deleted after.
				$trash = ROOT . '/.trash-' . bin2hex( random_bytes( 6 ) );
				if ( @rename( $base, $trash ) ) {
					remove_tree( $trash );
				}
				continue;
			}
			if ( $state['home'] ) {
				@unlink( $base . '/index.html' );
				@unlink( $base . '/index-noslash.html' );
				remove_tree( $base . '/page' );
			}
			foreach ( array_keys( $state['paths'] ) as $path ) {
				remove_tree( $base . '/' . $path );
			}
		}
	} catch ( \Throwable $e ) {
	}
}

function remove_tree( string $dir ): void {
	if ( strncmp( $dir, ROOT . '/', \strlen( ROOT ) + 1 ) !== 0 || strpos( $dir, '..' ) !== false || is_link( $dir ) || ! is_dir( $dir ) ) {
		return;
	}
	$items = new \RecursiveIteratorIterator( new \RecursiveDirectoryIterator( $dir, \FilesystemIterator::SKIP_DOTS ), \RecursiveIteratorIterator::CHILD_FIRST );
	foreach ( $items as $item ) {
		$item->isDir() && ! $item->isLink() ? @rmdir( $item->getPathname() ) : @unlink( $item->getPathname() );
	}
	@rmdir( $dir );
}
