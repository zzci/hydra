// Copyright © 2022 Ory Corp
// SPDX-License-Identifier: Apache-2.0

package oauth2_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"gopkg.in/square/go-jose.v2"

	"github.com/ory/fosite/token/jwt"
	"github.com/ory/hydra/jwk"
	"github.com/ory/hydra/oauth2/trust"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goauth2 "golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/ory/hydra/internal/testhelpers"
	"github.com/ory/x/contextx"

	hc "github.com/ory/hydra/client"
	"github.com/ory/hydra/driver/config"
	"github.com/ory/hydra/internal"
	"github.com/ory/hydra/x"
)

func TestJWTBearer(t *testing.T) {
	ctx := context.Background()
	reg := internal.NewMockedRegistry(t, &contextx.Default{})
	reg.Config().MustSet(ctx, config.KeyAccessTokenStrategy, "opaque")
	_, admin := testhelpers.NewOAuth2Server(ctx, t, reg)

	secret := uuid.New().String()
	client := &hc.Client{
		Secret:     secret,
		GrantTypes: []string{"client_credentials", "urn:ietf:params:oauth:grant-type:jwt-bearer"},
		Scope:      "offline_access",
	}
	require.NoError(t, reg.ClientManager().CreateClient(ctx, client))

	newConf := func(client *hc.Client) *clientcredentials.Config {
		return &clientcredentials.Config{
			ClientID:       client.GetID(),
			ClientSecret:   secret,
			TokenURL:       reg.Config().OAuth2TokenURL(ctx).String(),
			Scopes:         strings.Split(client.Scope, " "),
			EndpointParams: url.Values{"audience": client.Audience},
		}
	}

	var getToken = func(t *testing.T, conf *clientcredentials.Config) (*goauth2.Token, error) {
		conf.AuthStyle = goauth2.AuthStyleInHeader
		return conf.Token(context.Background())
	}

	var inspectToken = func(t *testing.T, token *goauth2.Token, cl *hc.Client, strategy string, grant trust.Grant) {
		introspection := testhelpers.IntrospectToken(t, &goauth2.Config{ClientID: cl.GetID(), ClientSecret: cl.Secret}, token.AccessToken, admin)

		check := func(res gjson.Result) {
			assert.EqualValues(t, cl.GetID(), res.Get("client_id").String(), "%s", res.Raw)
			assert.EqualValues(t, grant.Subject, res.Get("sub").String(), "%s", res.Raw)
			assert.EqualValues(t, reg.Config().IssuerURL(ctx).String(), res.Get("iss").String(), "%s", res.Raw)

			assert.EqualValues(t, res.Get("nbf").Int(), res.Get("iat").Int(), "%s", res.Raw)
			assert.True(t, res.Get("exp").Int() >= res.Get("iat").Int()+int64(reg.Config().GetAccessTokenLifespan(ctx).Seconds()), "%s", res.Raw)

			assert.EqualValues(t, fmt.Sprintf(`["%s"]`, reg.Config().OAuth2TokenURL(ctx).String()), res.Get("aud").Raw, "%s", res.Raw)
		}

		check(introspection)
		assert.True(t, introspection.Get("active").Bool())
		assert.EqualValues(t, "access_token", introspection.Get("token_use").String())
		assert.EqualValues(t, "Bearer", introspection.Get("token_type").String())
		assert.EqualValues(t, "offline_access", introspection.Get("scope").String(), "%s", introspection.Raw)

		if strategy != "jwt" {
			return
		}

		body, err := x.DecodeSegment(strings.Split(token.AccessToken, ".")[1])
		require.NoError(t, err)
		jwtClaims := gjson.ParseBytes(body)
		assert.NotEmpty(t, jwtClaims.Get("jti").String())
		assert.NotEmpty(t, jwtClaims.Get("iss").String())
		assert.NotEmpty(t, jwtClaims.Get("client_id").String())
		assert.EqualValues(t, "offline_access", introspection.Get("scope").String(), "%s", introspection.Raw)

		header, err := x.DecodeSegment(strings.Split(token.AccessToken, ".")[0])
		require.NoError(t, err)
		jwtHeader := gjson.ParseBytes(header)
		assert.NotEmpty(t, jwtHeader.Get("kid").String())
		assert.EqualValues(t, "offline_access", introspection.Get("scope").String(), "%s", introspection.Raw)

		check(jwtClaims)
	}

	t.Run("case=unable to exchange invalid jwt", func(t *testing.T) {
		conf := newConf(client)
		conf.EndpointParams = url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {"not-a-jwt"}}
		_, err := getToken(t, conf)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Unable to parse JSON Web Token")
	})

	t.Run("case=unable to request grant if not set", func(t *testing.T) {
		client := &hc.Client{
			Secret:     secret,
			GrantTypes: []string{"client_credentials"},
			Scope:      "offline_access",
		}
		require.NoError(t, reg.ClientManager().CreateClient(ctx, client))

		conf := newConf(client)
		conf.EndpointParams = url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {"not-a-jwt"}}
		_, err := getToken(t, conf)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "urn:ietf:params:oauth:grant-type:jwt-bearer")
	})

	set, kid := uuid.NewString(), uuid.NewString()
	keys, err := jwk.GenerateJWK(ctx, jose.RS256, kid, "sig")
	require.NoError(t, err)
	trustGrant := trust.Grant{
		ID:              uuid.NewString(),
		Issuer:          set,
		Subject:         uuid.NewString(),
		AllowAnySubject: false,
		Scope:           []string{"offline_access"},
		ExpiresAt:       time.Now().Add(time.Hour),
		PublicKey:       trust.PublicKey{Set: set, KeyID: kid},
	}
	require.NoError(t, reg.GrantManager().CreateGrant(ctx, trustGrant, keys.Keys[0].Public()))
	signer := jwk.NewDefaultJWTSigner(reg.Config(), reg, set)
	signer.GetPrivateKey = func(ctx context.Context) (interface{}, error) {
		return keys.Keys[0], nil
	}

	t.Run("case=unable to exchange token with a non-allowed subject", func(t *testing.T) {
		token, _, err := signer.Generate(ctx, jwt.MapClaims{
			"jti": uuid.NewString(),
			"iss": trustGrant.Issuer,
			"sub": uuid.NewString(),
			"aud": reg.Config().OAuth2TokenURL(ctx).String(),
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Add(-time.Minute).Unix(),
		}, &jwt.Headers{Extra: map[string]interface{}{"kid": kid}})
		require.NoError(t, err)

		conf := newConf(client)
		conf.EndpointParams = url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {token}}
		_, err = getToken(t, conf)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "public key is required to check signature of JWT")
	})

	t.Run("case=unable to exchange token with non-allowed scope", func(t *testing.T) {
		token, _, err := signer.Generate(ctx, jwt.MapClaims{
			"jti": uuid.NewString(),
			"iss": trustGrant.Issuer,
			"sub": trustGrant.Subject,
			"aud": reg.Config().OAuth2TokenURL(ctx).String(),
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Add(-time.Minute).Unix(),
		}, &jwt.Headers{Extra: map[string]interface{}{"kid": kid}})
		require.NoError(t, err)

		conf := newConf(client)
		conf.Scopes = []string{"i_am_not_allowed"}
		conf.EndpointParams = url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {token}}
		_, err = getToken(t, conf)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "i_am_not_allowed")
	})

	t.Run("case=unable to exchange token with an unknown kid", func(t *testing.T) {
		token, _, err := signer.Generate(ctx, jwt.MapClaims{
			"jti": uuid.NewString(),
			"iss": trustGrant.Issuer,
			"sub": trustGrant.Subject,
			"aud": reg.Config().OAuth2TokenURL(ctx).String(),
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Add(-time.Minute).Unix(),
		}, &jwt.Headers{Extra: map[string]interface{}{"kid": uuid.NewString()}})
		require.NoError(t, err)

		conf := newConf(client)
		conf.EndpointParams = url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {token}}
		_, err = getToken(t, conf)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "public key is required to check signature of JWT")
	})

	t.Run("case=unable to exchange token with an invalid key", func(t *testing.T) {
		keys, err := jwk.GenerateJWK(ctx, jose.RS256, kid, "sig")
		require.NoError(t, err)
		signer := jwk.NewDefaultJWTSigner(reg.Config(), reg, set)
		signer.GetPrivateKey = func(ctx context.Context) (interface{}, error) {
			return keys.Keys[0], nil
		}

		token, _, err := signer.Generate(ctx, jwt.MapClaims{
			"jti": uuid.NewString(),
			"iss": trustGrant.Issuer,
			"sub": trustGrant.Subject,
			"aud": reg.Config().OAuth2TokenURL(ctx).String(),
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Add(-time.Minute).Unix(),
		}, &jwt.Headers{Extra: map[string]interface{}{"kid": kid}})
		require.NoError(t, err)

		conf := newConf(client)
		conf.EndpointParams = url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {token}}
		_, err = getToken(t, conf)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Unable to verify the integrity")
	})

	t.Run("case=should exchange for an access token", func(t *testing.T) {
		run := func(strategy string) func(t *testing.T) {
			return func(t *testing.T) {
				reg.Config().MustSet(ctx, config.KeyAccessTokenStrategy, strategy)

				token, _, err := signer.Generate(ctx, jwt.MapClaims{
					"jti": uuid.NewString(),
					"iss": trustGrant.Issuer,
					"sub": trustGrant.Subject,
					"aud": reg.Config().OAuth2TokenURL(ctx).String(),
					"exp": time.Now().Add(time.Hour).Unix(),
					"iat": time.Now().Add(-time.Minute).Unix(),
				}, &jwt.Headers{Extra: map[string]interface{}{"kid": kid}})
				require.NoError(t, err)

				conf := newConf(client)
				conf.EndpointParams = url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {token}}

				result, err := getToken(t, conf)
				require.NoError(t, err)

				inspectToken(t, result, client, strategy, trustGrant)
			}
		}

		t.Run("strategy=opaque", run("opaque"))
		t.Run("strategy=jwt", run("jwt"))
	})

	t.Run("case=exchange for an access token without client", func(t *testing.T) {
		t.Skip("This currently does not work because the client is a required foreign key and also required throughout the code base.")

		run := func(strategy string) func(t *testing.T) {
			return func(t *testing.T) {
				reg.Config().MustSet(ctx, config.KeyAccessTokenStrategy, strategy)
				reg.Config().MustSet(ctx, "config.KeyOAuth2GrantJWTClientAuthOptional", true)

				token, _, err := signer.Generate(ctx, jwt.MapClaims{
					"jti": uuid.NewString(),
					"iss": trustGrant.Issuer,
					"sub": trustGrant.Subject,
					"aud": reg.Config().OAuth2TokenURL(ctx).String(),
					"exp": time.Now().Add(time.Hour).Unix(),
					"iat": time.Now().Add(-time.Minute).Unix(),
				}, &jwt.Headers{Extra: map[string]interface{}{"kid": kid}})
				require.NoError(t, err)

				res, err := http.DefaultClient.PostForm(reg.Config().OAuth2TokenURL(ctx).String(), url.Values{
					"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
					"assertion":  {token},
				})
				require.NoError(t, err)
				defer res.Body.Close()
				body, err := io.ReadAll(res.Body)
				require.NoError(t, err)
				require.EqualValues(t, http.StatusOK, res.StatusCode, "%s", body)

				var result goauth2.Token
				require.NoError(t, json.Unmarshal(body, &result))
				assert.NotEmpty(t, result.AccessToken, "%s", body)

				inspectToken(t, &result, client, strategy, trustGrant)
			}
		}

		t.Run("strategy=opaque", run("opaque"))
		t.Run("strategy=jwt", run("jwt"))
	})
}
