// Package sonarrclient constructs a generated Sonarr API client
// (openapi/sonarr.gen.go, produced by oapi-codegen from
// openapi/sonarr_openapi.yaml) wired to use CachedHTTP as its HTTP
// transport, so repeated GETs against the same Sonarr instance are served
// from Redis instead of re-hitting Sonarr within the cache TTL.
package sonarrclient

import (
	"github.com/oapi-codegen/oapi-codegen/v2/pkg/securityprovider"
	"github.com/redis/go-redis/v9"

	"Metarr/internal/httpclient"
	"Metarr/openapi"
)

// New builds a typed client for the Sonarr instance at baseURL,
// authenticating every request with apiKey (sent as the X-Api-Key header,
// per the Sonarr OpenAPI spec's security scheme) and caching GET responses
// in redisClient via CachedHTTP.
func New(baseURL, apiKey string, redisClient *redis.Client) (*interfaces.ClientWithResponses, error) {
	apiKeyAuth, err := securityprovider.NewSecurityProviderApiKey("header", "X-Api-Key", apiKey)
	if err != nil {
		return nil, err
	}

	return interfaces.NewClientWithResponses(
		baseURL,
		interfaces.WithHTTPClient(httpclient.New(nil, redisClient)),
		interfaces.WithRequestEditorFn(apiKeyAuth.Intercept),
	)
}
