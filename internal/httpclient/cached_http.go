// Package httpclient provides an http.Client wrapper that transparently
// caches GET responses in Redis.
package httpclient

import (
	"bufio"
	"bytes"
	"context"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/redis/go-redis/v9"
)

// DefaultTTL is how long a cached GET response is kept in Redis before the
// next request for that URI goes back out to the network.
const DefaultTTL = 30 * time.Minute

const cacheKeyPrefix = "httpcache:"

// CachedHTTP wraps http.Client, serving GET requests from a Redis cache
// when a cached response exists and populating the cache when it doesn't.
// Every other HTTP method passes straight through to the underlying
// client, uncached.
type CachedHTTP struct {
	*http.Client
	redis *redis.Client
	ttl   time.Duration
}

// New wraps client (or http.DefaultClient if nil) with Redis-backed GET
// caching using DefaultTTL.
func New(client *http.Client, redisClient *redis.Client) *CachedHTTP {
	if client == nil {
		client = http.DefaultClient
	}
	return &CachedHTTP{
		Client: client,
		redis:  redisClient,
		ttl:    DefaultTTL,
	}
}

// Get issues a GET request for url, serving a cached response from Redis
// when one exists instead of hitting the network.
func (c *CachedHTTP) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Do sends req like the embedded http.Client would, except GET requests
// are checked against (and used to populate) the Redis cache first.
func (c *CachedHTTP) Do(req *http.Request) (*http.Response, error) {
	if req.Method != "" && req.Method != http.MethodGet {
		return c.Client.Do(req)
	}

	ctx := req.Context()
	key := cacheKeyPrefix + req.URL.String()

	if resp, err := c.readCached(ctx, key, req); err == nil {
		return resp, nil
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}

	c.cache(ctx, key, resp)
	return resp, nil
}

// readCached returns the cached response for key, or an error if it's not
// present (or Redis can't be reached) so the caller falls back to a live
// request.
func (c *CachedHTTP) readCached(ctx context.Context, key string, req *http.Request) (*http.Response, error) {
	raw, err := c.redis.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	return http.ReadResponse(bufio.NewReader(bytes.NewReader(raw)), req)
}

// cache stores a wire-format dump of resp in Redis under key with the
// configured TTL. httputil.DumpResponse drains and restores resp.Body in
// the process, so resp remains readable by the caller afterward. Only
// successful (2xx) responses are cached; if dumping or the Redis write
// fails, the live response is still returned to the caller regardless.
func (c *CachedHTTP) cache(ctx context.Context, key string, resp *http.Response) {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return
	}

	dump, err := httputil.DumpResponse(resp, true)
	if err != nil {
		return
	}

	c.redis.Set(ctx, key, dump, c.ttl)
}
