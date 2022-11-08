package driver

import (
	"context"

	"github.com/ory/x/httprouterx"

	"github.com/ory/hydra/hsm"
	"github.com/ory/x/contextx"

	"github.com/ory/hydra/oauth2/trust"

	"github.com/pkg/errors"

	"github.com/ory/x/errorsx"

	"github.com/ory/fosite"
	foauth2 "github.com/ory/fosite/handler/oauth2"

	"github.com/ory/x/logrusx"

	"github.com/ory/hydra/persistence"

	prometheus "github.com/ory/x/prometheusx"

	"github.com/ory/x/dbal"
	"github.com/ory/x/healthx"

	"github.com/ory/hydra/client"
	"github.com/ory/hydra/consent"
	"github.com/ory/hydra/driver/config"
	"github.com/ory/hydra/jwk"
	"github.com/ory/hydra/oauth2"
	"github.com/ory/hydra/x"
)

type Registry interface {
	dbal.Driver

	Init(ctx context.Context, skipNetworkInit bool, migrate bool, ctxer contextx.Contextualizer) error

	WithBuildInfo(v, h, d string) Registry
	WithConfig(c *config.DefaultProvider) Registry
	WithContextualizer(ctxer contextx.Contextualizer) Registry
	WithLogger(l *logrusx.Logger) Registry
	x.HTTPClientProvider
	GetJWKSFetcherStrategy() fosite.JWKSFetcherStrategy

	config.Provider
	persistence.Provider
	x.RegistryLogger
	x.RegistryWriter
	x.RegistryCookieStore
	client.Registry
	consent.Registry
	jwk.Registry
	trust.Registry
	oauth2.Registry
	PrometheusManager() *prometheus.MetricsManager
	x.TracingProvider

	RegisterRoutes(ctx context.Context, admin *httprouterx.RouterAdmin, public *httprouterx.RouterPublic)
	ClientHandler() *client.Handler
	KeyHandler() *jwk.Handler
	ConsentHandler() *consent.Handler
	OAuth2Handler() *oauth2.Handler
	HealthHandler() *healthx.Handler

	OAuth2HMACStrategy() *foauth2.HMACSHAStrategy
	WithOAuth2Provider(f fosite.OAuth2Provider)
	WithConsentStrategy(c consent.Strategy)
	WithHsmContext(h hsm.Context)
}

func NewRegistryFromDSN(ctx context.Context, c *config.DefaultProvider, l *logrusx.Logger, skipNetworkInit bool, migrate bool, ctxer contextx.Contextualizer) (Registry, error) {
	registry, err := NewRegistryWithoutInit(c, l)
	if err != nil {
		return nil, err
	}
	if err := registry.Init(ctx, skipNetworkInit, migrate, ctxer); err != nil {
		return nil, err
	}
	return registry, nil
}

func NewRegistryWithoutInit(c *config.DefaultProvider, l *logrusx.Logger) (Registry, error) {
	driver, err := dbal.GetDriverFor(c.DSN())
	if err != nil {
		return nil, errorsx.WithStack(err)
	}
	registry, ok := driver.(Registry)
	if !ok {
		return nil, errors.Errorf("driver of type %T does not implement interface Registry", driver)
	}
	registry = registry.WithLogger(l).WithConfig(c).WithBuildInfo(config.Version, config.Commit, config.Date)

	return registry, nil
}

func CallRegistry(ctx context.Context, r Registry) {
	r.ClientValidator()
	r.ClientManager()
	r.ClientHasher()
	r.ConsentManager()
	r.ConsentStrategy()
	r.SubjectIdentifierAlgorithm(ctx)
	r.KeyManager()
	r.KeyCipher()
	r.OAuth2Storage()
	r.OAuth2Provider()
	r.AudienceStrategy()
	r.AccessTokenJWTStrategy()
	r.OpenIDJWTStrategy()
	r.OpenIDConnectRequestValidator()
	r.PrometheusManager()
	r.Tracer(ctx)
}
