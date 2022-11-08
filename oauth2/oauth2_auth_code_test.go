/*
 * Copyright © 2015-2018 Aeneas Rekkas <aeneas+oss@aeneas.io>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * @author		Aeneas Rekkas <aeneas+oss@aeneas.io>
 * @copyright 	2015-2018 Aeneas Rekkas <aeneas+oss@aeneas.io>
 * @license 	Apache-2.0
 */

package oauth2_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ory/x/ioutilx"
	"github.com/ory/x/requirex"

	hydra "github.com/ory/hydra-client-go"

	"github.com/ory/x/httprouterx"

	"github.com/ory/x/assertx"

	"github.com/pborman/uuid"
	"github.com/tidwall/gjson"

	"github.com/ory/hydra/client"
	"github.com/ory/hydra/consent"
	"github.com/ory/hydra/internal/testhelpers"
	"github.com/ory/x/contextx"

	"github.com/julienschmidt/httprouter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	goauth2 "golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/ory/fosite"
	hc "github.com/ory/hydra/client"
	"github.com/ory/hydra/driver/config"
	"github.com/ory/hydra/internal"
	hydraoauth2 "github.com/ory/hydra/oauth2"
	"github.com/ory/hydra/x"
	"github.com/ory/x/pointerx"
	"github.com/ory/x/snapshotx"
)

func noopHandler(t *testing.T) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		w.WriteHeader(http.StatusNotImplemented)
	}
}

type clientCreator interface {
	CreateClient(cxt context.Context, client *hc.Client) error
}

// TestAuthCodeWithDefaultStrategy runs proper integration tests against in-memory and database connectors, specifically
// we test:
//
// - [x] If the flow - in general - works
// - [x] If `authenticatedAt` is properly managed across the lifecycle
//   - [x] The value `authenticatedAt` should be an old time if no user interaction wrt login was required
//   - [x] The value `authenticatedAt` should be a recent time if user interaction wrt login was required
//
// - [x] If `requestedAt` is properly managed across the lifecycle
//   - [x] The value of `requestedAt` must be the initial request time, not some other time (e.g. when accepting login)
//
// - [x] If `id_token_hint` is handled properly
//   - [x] What happens if `id_token_hint` does not match the value from the handled authentication request ("accept login")
func TestAuthCodeWithDefaultStrategy(t *testing.T) {
	ctx := context.TODO()
	reg := internal.NewMockedRegistry(t, &contextx.Default{})
	reg.Config().MustSet(ctx, config.KeyAccessTokenStrategy, "opaque")
	reg.Config().MustSet(ctx, config.KeyRefreshTokenHookURL, "")
	publicTS, adminTS := testhelpers.NewOAuth2Server(ctx, t, reg)

	newOAuth2Client := func(t *testing.T, cb string) (*hc.Client, *oauth2.Config) {
		secret := uuid.New()
		c := &hc.Client{
			Secret:        secret,
			RedirectURIs:  []string{cb},
			ResponseTypes: []string{"id_token", "code", "token"},
			GrantTypes:    []string{"implicit", "refresh_token", "authorization_code", "password", "client_credentials"},
			Scope:         "hydra offline openid",
			Audience:      []string{"https://api.ory.sh/"},
		}
		require.NoError(t, reg.ClientManager().CreateClient(context.TODO(), c))
		return c, &oauth2.Config{
			ClientID:     c.GetID(),
			ClientSecret: secret,
			Endpoint: oauth2.Endpoint{
				AuthURL:   reg.Config().OAuth2AuthURL(ctx).String(),
				TokenURL:  reg.Config().OAuth2TokenURL(ctx).String(),
				AuthStyle: oauth2.AuthStyleInHeader,
			},
			Scopes: strings.Split(c.Scope, " "),
		}
	}

	adminClient := hydra.NewAPIClient(hydra.NewConfiguration())
	adminClient.GetConfig().Servers = hydra.ServerConfigurations{{URL: adminTS.URL}}

	getAuthorizeCode := func(t *testing.T, conf *oauth2.Config, c *http.Client, params ...oauth2.AuthCodeOption) (string, *http.Response) {
		if c == nil {
			c = testhelpers.NewEmptyJarClient(t)
		}

		state := uuid.New()
		resp, err := c.Get(conf.AuthCodeURL(state, params...))
		require.NoError(t, err)
		defer resp.Body.Close()

		q := resp.Request.URL.Query()
		require.EqualValues(t, state, q.Get("state"))
		return q.Get("code"), resp
	}

	acceptLoginHandler := func(t *testing.T, c *client.Client, subject string, checkRequestPayload func(request *hydra.OAuth2LoginRequest) *hydra.AcceptOAuth2LoginRequest) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			rr, _, err := adminClient.OAuth2Api.GetOAuth2LoginRequest(context.Background()).LoginChallenge(r.URL.Query().Get("login_challenge")).Execute()
			require.NoError(t, err)

			assert.EqualValues(t, c.GetID(), pointerx.StringR(rr.Client.ClientId))
			assert.Empty(t, pointerx.StringR(rr.Client.ClientSecret))
			assert.EqualValues(t, c.GrantTypes, rr.Client.GrantTypes)
			assert.EqualValues(t, c.LogoURI, pointerx.StringR(rr.Client.LogoUri))
			assert.EqualValues(t, c.RedirectURIs, rr.Client.RedirectUris)
			assert.EqualValues(t, r.URL.Query().Get("login_challenge"), rr.Challenge)
			assert.EqualValues(t, []string{"hydra", "offline", "openid"}, rr.RequestedScope)
			assert.Contains(t, rr.RequestUrl, reg.Config().OAuth2AuthURL(ctx).String())

			acceptBody := hydra.AcceptOAuth2LoginRequest{
				Subject:  subject,
				Remember: pointerx.Bool(!rr.Skip),
				Acr:      pointerx.String("1"),
				Amr:      []string{"pwd"},
				Context:  map[string]interface{}{"context": "bar"},
			}
			if checkRequestPayload != nil {
				if b := checkRequestPayload(rr); b != nil {
					acceptBody = *b
				}
			}

			v, _, err := adminClient.OAuth2Api.AcceptOAuth2LoginRequest(context.Background()).
				LoginChallenge(r.URL.Query().Get("login_challenge")).
				AcceptOAuth2LoginRequest(acceptBody).
				Execute()
			require.NoError(t, err)
			require.NotEmpty(t, v.RedirectTo)
			http.Redirect(w, r, v.RedirectTo, http.StatusFound)
		}
	}

	acceptConsentHandler := func(t *testing.T, c *client.Client, subject string, checkRequestPayload func(*hydra.OAuth2ConsentRequest)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			rr, _, err := adminClient.OAuth2Api.GetOAuth2ConsentRequest(context.Background()).ConsentChallenge(r.URL.Query().Get("consent_challenge")).Execute()
			require.NoError(t, err)

			assert.EqualValues(t, c.GetID(), pointerx.StringR(rr.Client.ClientId))
			assert.Empty(t, pointerx.StringR(rr.Client.ClientSecret))
			assert.EqualValues(t, c.GrantTypes, rr.Client.GrantTypes)
			assert.EqualValues(t, c.LogoURI, pointerx.StringR(rr.Client.LogoUri))
			assert.EqualValues(t, c.RedirectURIs, rr.Client.RedirectUris)
			assert.EqualValues(t, subject, pointerx.StringR(rr.Subject))
			assert.EqualValues(t, []string{"hydra", "offline", "openid"}, rr.RequestedScope)
			assert.EqualValues(t, r.URL.Query().Get("consent_challenge"), rr.Challenge)
			assert.Contains(t, *rr.RequestUrl, reg.Config().OAuth2AuthURL(ctx).String())
			if checkRequestPayload != nil {
				checkRequestPayload(rr)
			}

			assert.Equal(t, map[string]interface{}{"context": "bar"}, rr.Context)
			v, _, err := adminClient.OAuth2Api.AcceptOAuth2ConsentRequest(context.Background()).
				ConsentChallenge(r.URL.Query().Get("consent_challenge")).
				AcceptOAuth2ConsentRequest(hydra.AcceptOAuth2ConsentRequest{
					GrantScope: []string{"hydra", "offline", "openid"}, Remember: pointerx.Bool(true), RememberFor: pointerx.Int64(0),
					GrantAccessTokenAudience: rr.RequestedAccessTokenAudience,
					Session: &hydra.AcceptOAuth2ConsentRequestSession{
						AccessToken: map[string]interface{}{"foo": "bar"},
						IdToken:     map[string]interface{}{"bar": "baz"},
					},
				}).
				Execute()
			require.NoError(t, err)
			require.NotEmpty(t, v.RedirectTo)
			http.Redirect(w, r, v.RedirectTo, http.StatusFound)
		}
	}

	assertRefreshToken := func(t *testing.T, token *oauth2.Token, c *oauth2.Config, expectedExp time.Time) {
		actualExp, err := strconv.ParseInt(testhelpers.IntrospectToken(t, c, token.RefreshToken, adminTS).Get("exp").String(), 10, 64)
		require.NoError(t, err)
		requirex.EqualTime(t, expectedExp, time.Unix(actualExp, 0), time.Second)
	}

	assertIDToken := func(t *testing.T, token *oauth2.Token, c *oauth2.Config, expectedSubject, expectedNonce string, expectedExp time.Time) gjson.Result {
		idt, ok := token.Extra("id_token").(string)
		require.True(t, ok)
		assert.NotEmpty(t, idt)

		body, err := x.DecodeSegment(strings.Split(idt, ".")[1])
		require.NoError(t, err)

		claims := gjson.ParseBytes(body)
		assert.True(t, time.Now().After(time.Unix(claims.Get("iat").Int(), 0)), "%s", claims)
		assert.True(t, time.Now().After(time.Unix(claims.Get("nbf").Int(), 0)), "%s", claims)
		assert.True(t, time.Now().Before(time.Unix(claims.Get("exp").Int(), 0)), "%s", claims)
		requirex.EqualTime(t, expectedExp, time.Unix(claims.Get("exp").Int(), 0), 2*time.Second)
		assert.NotEmpty(t, claims.Get("jti").String(), "%s", claims)
		assert.EqualValues(t, reg.Config().IssuerURL(ctx).String(), claims.Get("iss").String(), "%s", claims)
		assert.NotEmpty(t, claims.Get("sid").String(), "%s", claims)
		assert.Equal(t, "1", claims.Get("acr").String(), "%s", claims)
		require.Len(t, claims.Get("amr").Array(), 1, "%s", claims)
		assert.EqualValues(t, "pwd", claims.Get("amr").Array()[0].String(), "%s", claims)

		require.Len(t, claims.Get("aud").Array(), 1, "%s", claims)
		assert.EqualValues(t, c.ClientID, claims.Get("aud").Array()[0].String(), "%s", claims)
		assert.EqualValues(t, expectedSubject, claims.Get("sub").String(), "%s", claims)
		assert.EqualValues(t, expectedNonce, claims.Get("nonce").String(), "%s", claims)
		assert.EqualValues(t, `baz`, claims.Get("bar").String(), "%s", claims)

		return claims
	}

	introspectAccessToken := func(t *testing.T, conf *oauth2.Config, token *oauth2.Token, expectedSubject string) gjson.Result {
		require.NotEmpty(t, token.AccessToken)
		i := testhelpers.IntrospectToken(t, conf, token.AccessToken, adminTS)
		assert.True(t, i.Get("active").Bool(), "%s", i)
		assert.EqualValues(t, conf.ClientID, i.Get("client_id").String(), "%s", i)
		assert.EqualValues(t, expectedSubject, i.Get("sub").String(), "%s", i)
		assert.EqualValues(t, `{"foo":"bar"}`, i.Get("ext").Raw, "%s", i)
		return i
	}

	assertJWTAccessToken := func(t *testing.T, strat string, conf *oauth2.Config, token *oauth2.Token, expectedSubject string, expectedExp time.Time) gjson.Result {
		require.NotEmpty(t, token.AccessToken)
		parts := strings.Split(token.AccessToken, ".")
		if strat != "jwt" {
			require.Len(t, parts, 2)
			return gjson.Parse("null")
		}
		require.Len(t, parts, 3)

		body, err := x.DecodeSegment(parts[1])
		require.NoError(t, err)

		i := gjson.ParseBytes(body)
		assert.NotEmpty(t, i.Get("jti").String())
		assert.EqualValues(t, conf.ClientID, i.Get("client_id").String(), "%s", i)
		assert.EqualValues(t, expectedSubject, i.Get("sub").String(), "%s", i)
		assert.EqualValues(t, reg.Config().IssuerURL(ctx).String(), i.Get("iss").String(), "%s", i)
		assert.True(t, time.Now().After(time.Unix(i.Get("iat").Int(), 0)), "%s", i)
		assert.True(t, time.Now().After(time.Unix(i.Get("nbf").Int(), 0)), "%s", i)
		assert.True(t, time.Now().Before(time.Unix(i.Get("exp").Int(), 0)), "%s", i)
		requirex.EqualTime(t, expectedExp, time.Unix(i.Get("exp").Int(), 0), time.Second)
		assert.EqualValues(t, `{"foo":"bar"}`, i.Get("ext").Raw, "%s", i)
		assert.EqualValues(t, `["hydra","offline","openid"]`, i.Get("scp").Raw, "%s", i)
		return i
	}

	waitForRefreshTokenExpiry := func() {
		time.Sleep(reg.Config().GetRefreshTokenLifespan(ctx) + time.Second)
	}

	t.Run("case=checks if request fails when audience does not match", func(t *testing.T) {
		testhelpers.NewLoginConsentUI(t, reg.Config(), testhelpers.HTTPServerNoExpectedCallHandler(t), testhelpers.HTTPServerNoExpectedCallHandler(t))
		_, conf := newOAuth2Client(t, testhelpers.NewCallbackURL(t, "callback", testhelpers.HTTPServerNotImplementedHandler))
		code, _ := getAuthorizeCode(t, conf, nil, oauth2.SetAuthURLParam("audience", "https://not-ory-api/"))
		require.Empty(t, code)
	})

	subject := "aeneas-rekkas"
	nonce := uuid.New()
	t.Run("case=perform authorize code flow with ID token and refresh tokens", func(t *testing.T) {
		run := func(t *testing.T, strategy string) {
			c, conf := newOAuth2Client(t, testhelpers.NewCallbackURL(t, "callback", testhelpers.HTTPServerNotImplementedHandler))
			testhelpers.NewLoginConsentUI(t, reg.Config(),
				acceptLoginHandler(t, c, subject, nil),
				acceptConsentHandler(t, c, subject, nil),
			)

			code, _ := getAuthorizeCode(t, conf, nil, oauth2.SetAuthURLParam("nonce", nonce))
			require.NotEmpty(t, code)
			token, err := conf.Exchange(context.Background(), code)
			iat := time.Now()
			require.NoError(t, err)

			introspectAccessToken(t, conf, token, subject)
			assertJWTAccessToken(t, strategy, conf, token, subject, iat.Add(reg.Config().GetAccessTokenLifespan(ctx)))
			assertIDToken(t, token, conf, subject, nonce, iat.Add(reg.Config().GetIDTokenLifespan(ctx)))
			assertRefreshToken(t, token, conf, iat.Add(reg.Config().GetRefreshTokenLifespan(ctx)))

			t.Run("followup=successfully perform refresh token flow", func(t *testing.T) {
				require.NotEmpty(t, token.RefreshToken)
				token.Expiry = token.Expiry.Add(-time.Hour * 24)
				iat = time.Now()
				refreshedToken, err := conf.TokenSource(context.Background(), token).Token()
				require.NoError(t, err)

				require.NotEqual(t, token.AccessToken, refreshedToken.AccessToken)
				require.NotEqual(t, token.RefreshToken, refreshedToken.RefreshToken)
				require.NotEqual(t, token.Extra("id_token"), refreshedToken.Extra("id_token"))
				introspectAccessToken(t, conf, refreshedToken, subject)

				t.Run("followup=refreshed tokens contain valid tokens", func(t *testing.T) {
					assertJWTAccessToken(t, strategy, conf, refreshedToken, subject, iat.Add(reg.Config().GetAccessTokenLifespan(ctx)))
					assertIDToken(t, refreshedToken, conf, subject, nonce, iat.Add(reg.Config().GetIDTokenLifespan(ctx)))
					assertRefreshToken(t, refreshedToken, conf, iat.Add(reg.Config().GetRefreshTokenLifespan(ctx)))
				})

				t.Run("followup=original access token is no longer valid", func(t *testing.T) {
					i := testhelpers.IntrospectToken(t, conf, token.AccessToken, adminTS)
					assert.False(t, i.Get("active").Bool(), "%s", i)
				})

				t.Run("followup=original refresh token is no longer valid", func(t *testing.T) {
					_, err := conf.TokenSource(context.Background(), token).Token()
					assert.Error(t, err)
				})

				t.Run("followup=but fail subsequent refresh because expiry was reached", func(t *testing.T) {
					waitForRefreshTokenExpiry()

					// Force golang to refresh token
					refreshedToken.Expiry = refreshedToken.Expiry.Add(-time.Hour * 24)
					_, err := conf.TokenSource(context.Background(), refreshedToken).Token()
					require.Error(t, err)
				})
			})
		}

		t.Run("strategy=jwt", func(t *testing.T) {
			reg.Config().MustSet(ctx, config.KeyAccessTokenStrategy, "jwt")
			run(t, "jwt")
		})

		t.Run("strategy=opaque", func(t *testing.T) {
			reg.Config().MustSet(ctx, config.KeyAccessTokenStrategy, "opaque")
			run(t, "opaque")
		})
	})

	t.Run("case=checks if request fails when subject is empty", func(t *testing.T) {
		testhelpers.NewLoginConsentUI(t, reg.Config(), func(w http.ResponseWriter, r *http.Request) {
			_, res, err := adminClient.OAuth2Api.AcceptOAuth2LoginRequest(ctx).
				LoginChallenge(r.URL.Query().Get("login_challenge")).
				AcceptOAuth2LoginRequest(hydra.AcceptOAuth2LoginRequest{Subject: "", Remember: pointerx.Bool(true)}).Execute()
			require.Error(t, err) // expects 400
			body := string(ioutilx.MustReadAll(res.Body))
			assert.Contains(t, body, "Field 'subject' must not be empty", "%s", body)
		}, testhelpers.HTTPServerNoExpectedCallHandler(t))
		_, conf := newOAuth2Client(t, testhelpers.NewCallbackURL(t, "callback", testhelpers.HTTPServerNotImplementedHandler))

		_, err := testhelpers.NewEmptyJarClient(t).Get(conf.AuthCodeURL(uuid.New()))
		require.NoError(t, err)
	})

	t.Run("case=perform flow with audience", func(t *testing.T) {
		expectAud := "https://api.ory.sh/"
		c, conf := newOAuth2Client(t, testhelpers.NewCallbackURL(t, "callback", testhelpers.HTTPServerNotImplementedHandler))
		testhelpers.NewLoginConsentUI(t, reg.Config(),
			acceptLoginHandler(t, c, subject, func(r *hydra.OAuth2LoginRequest) *hydra.AcceptOAuth2LoginRequest {
				assert.False(t, r.Skip)
				assert.EqualValues(t, []string{expectAud}, r.RequestedAccessTokenAudience)
				return nil
			}),
			acceptConsentHandler(t, c, subject, func(r *hydra.OAuth2ConsentRequest) {
				assert.False(t, *r.Skip)
				assert.EqualValues(t, []string{expectAud}, r.RequestedAccessTokenAudience)
			}))

		code, _ := getAuthorizeCode(t, conf, nil,
			oauth2.SetAuthURLParam("audience", "https://api.ory.sh/"),
			oauth2.SetAuthURLParam("nonce", nonce))
		require.NotEmpty(t, code)

		token, err := conf.Exchange(context.Background(), code)
		require.NoError(t, err)

		claims := introspectAccessToken(t, conf, token, subject)
		aud := claims.Get("aud").Array()
		require.Len(t, aud, 1)
		assert.EqualValues(t, aud[0].String(), expectAud)

		assertIDToken(t, token, conf, subject, nonce, time.Now().Add(reg.Config().GetIDTokenLifespan(ctx)))
	})

	t.Run("case=respects client token lifespan configuration", func(t *testing.T) {
		run := func(t *testing.T, strategy string, c *hc.Client, conf *oauth2.Config, expectedLifespans client.Lifespans) {
			testhelpers.NewLoginConsentUI(t, reg.Config(),
				acceptLoginHandler(t, c, subject, nil),
				acceptConsentHandler(t, c, subject, nil),
			)

			code, _ := getAuthorizeCode(t, conf, nil, oauth2.SetAuthURLParam("nonce", nonce))
			require.NotEmpty(t, code)
			token, err := conf.Exchange(context.Background(), code)
			iat := time.Now()
			require.NoError(t, err)

			body := introspectAccessToken(t, conf, token, subject)
			requirex.EqualTime(t, iat.Add(expectedLifespans.AuthorizationCodeGrantAccessTokenLifespan.Duration), time.Unix(body.Get("exp").Int(), 0), time.Second)

			assertJWTAccessToken(t, strategy, conf, token, subject, iat.Add(expectedLifespans.AuthorizationCodeGrantAccessTokenLifespan.Duration))
			assertIDToken(t, token, conf, subject, nonce, iat.Add(expectedLifespans.AuthorizationCodeGrantIDTokenLifespan.Duration))
			assertRefreshToken(t, token, conf, iat.Add(expectedLifespans.AuthorizationCodeGrantRefreshTokenLifespan.Duration))

			t.Run("followup=successfully perform refresh token flow", func(t *testing.T) {
				require.NotEmpty(t, token.RefreshToken)
				token.Expiry = token.Expiry.Add(-time.Hour * 24)
				refreshedToken, err := conf.TokenSource(context.Background(), token).Token()
				iat = time.Now()
				require.NoError(t, err)
				assertRefreshToken(t, refreshedToken, conf, iat.Add(expectedLifespans.RefreshTokenGrantRefreshTokenLifespan.Duration))
				assertJWTAccessToken(t, strategy, conf, refreshedToken, subject, iat.Add(expectedLifespans.RefreshTokenGrantAccessTokenLifespan.Duration))
				assertIDToken(t, refreshedToken, conf, subject, nonce, iat.Add(expectedLifespans.RefreshTokenGrantIDTokenLifespan.Duration))

				require.NotEqual(t, token.AccessToken, refreshedToken.AccessToken)
				require.NotEqual(t, token.RefreshToken, refreshedToken.RefreshToken)
				require.NotEqual(t, token.Extra("id_token"), refreshedToken.Extra("id_token"))

				body := introspectAccessToken(t, conf, refreshedToken, subject)
				requirex.EqualTime(t, iat.Add(expectedLifespans.RefreshTokenGrantAccessTokenLifespan.Duration), time.Unix(body.Get("exp").Int(), 0), time.Second)

				t.Run("followup=original access token is no longer valid", func(t *testing.T) {
					i := testhelpers.IntrospectToken(t, conf, token.AccessToken, adminTS)
					assert.False(t, i.Get("active").Bool(), "%s", i)
				})

				t.Run("followup=original refresh token is no longer valid", func(t *testing.T) {
					_, err := conf.TokenSource(context.Background(), token).Token()
					assert.Error(t, err)
				})
			})
		}

		t.Run("case=custom-lifespans-active-jwt", func(t *testing.T) {
			c, conf := newOAuth2Client(t, testhelpers.NewCallbackURL(t, "callback", testhelpers.HTTPServerNotImplementedHandler))
			ls := testhelpers.TestLifespans
			ls.AuthorizationCodeGrantAccessTokenLifespan = x.NullDuration{Valid: true, Duration: 6 * time.Second}
			testhelpers.UpdateClientTokenLifespans(
				t,
				&goauth2.Config{ClientID: c.GetID(), ClientSecret: conf.ClientSecret},
				c.GetID(),
				ls, adminTS,
			)
			reg.Config().MustSet(ctx, config.KeyAccessTokenStrategy, "jwt")
			run(t, "jwt", c, conf, ls)
		})

		t.Run("case=custom-lifespans-active-opaque", func(t *testing.T) {
			c, conf := newOAuth2Client(t, testhelpers.NewCallbackURL(t, "callback", testhelpers.HTTPServerNotImplementedHandler))
			ls := testhelpers.TestLifespans
			ls.AuthorizationCodeGrantAccessTokenLifespan = x.NullDuration{Valid: true, Duration: 6 * time.Second}
			testhelpers.UpdateClientTokenLifespans(
				t,
				&goauth2.Config{ClientID: c.GetID(), ClientSecret: conf.ClientSecret},
				c.GetID(),
				ls, adminTS,
			)
			reg.Config().MustSet(ctx, config.KeyAccessTokenStrategy, "opaque")
			run(t, "opaque", c, conf, ls)
		})

		t.Run("case=custom-lifespans-unset", func(t *testing.T) {
			c, conf := newOAuth2Client(t, testhelpers.NewCallbackURL(t, "callback", testhelpers.HTTPServerNotImplementedHandler))
			testhelpers.UpdateClientTokenLifespans(t, &goauth2.Config{ClientID: c.GetID(), ClientSecret: conf.ClientSecret}, c.GetID(), testhelpers.TestLifespans, adminTS)
			testhelpers.UpdateClientTokenLifespans(t, &goauth2.Config{ClientID: c.GetID(), ClientSecret: conf.ClientSecret}, c.GetID(), client.Lifespans{}, adminTS)
			reg.Config().MustSet(ctx, config.KeyAccessTokenStrategy, "opaque")

			expectedLifespans := client.Lifespans{
				AuthorizationCodeGrantAccessTokenLifespan:  x.NullDuration{Valid: true, Duration: reg.Config().GetAccessTokenLifespan(ctx)},
				AuthorizationCodeGrantIDTokenLifespan:      x.NullDuration{Valid: true, Duration: reg.Config().GetIDTokenLifespan(ctx)},
				AuthorizationCodeGrantRefreshTokenLifespan: x.NullDuration{Valid: true, Duration: reg.Config().GetRefreshTokenLifespan(ctx)},
				ClientCredentialsGrantAccessTokenLifespan:  x.NullDuration{Valid: true, Duration: reg.Config().GetAccessTokenLifespan(ctx)},
				ImplicitGrantAccessTokenLifespan:           x.NullDuration{Valid: true, Duration: reg.Config().GetAccessTokenLifespan(ctx)},
				ImplicitGrantIDTokenLifespan:               x.NullDuration{Valid: true, Duration: reg.Config().GetIDTokenLifespan(ctx)},
				JwtBearerGrantAccessTokenLifespan:          x.NullDuration{Valid: true, Duration: reg.Config().GetAccessTokenLifespan(ctx)},
				PasswordGrantAccessTokenLifespan:           x.NullDuration{Valid: true, Duration: reg.Config().GetAccessTokenLifespan(ctx)},
				PasswordGrantRefreshTokenLifespan:          x.NullDuration{Valid: true, Duration: reg.Config().GetRefreshTokenLifespan(ctx)},
				RefreshTokenGrantIDTokenLifespan:           x.NullDuration{Valid: true, Duration: reg.Config().GetIDTokenLifespan(ctx)},
				RefreshTokenGrantAccessTokenLifespan:       x.NullDuration{Valid: true, Duration: reg.Config().GetAccessTokenLifespan(ctx)},
				RefreshTokenGrantRefreshTokenLifespan:      x.NullDuration{Valid: true, Duration: reg.Config().GetRefreshTokenLifespan(ctx)},
			}
			run(t, "opaque", c, conf, expectedLifespans)
		})
	})

	t.Run("case=use remember feature and prompt=none", func(t *testing.T) {
		c, conf := newOAuth2Client(t, testhelpers.NewCallbackURL(t, "callback", testhelpers.HTTPServerNotImplementedHandler))
		testhelpers.NewLoginConsentUI(t, reg.Config(),
			acceptLoginHandler(t, c, subject, nil),
			acceptConsentHandler(t, c, subject, nil),
		)

		oc := testhelpers.NewEmptyJarClient(t)
		code, _ := getAuthorizeCode(t, conf, oc,
			oauth2.SetAuthURLParam("nonce", nonce),
			oauth2.SetAuthURLParam("prompt", "login consent"),
			oauth2.SetAuthURLParam("max_age", "1"),
		)
		require.NotEmpty(t, code)
		token, err := conf.Exchange(context.Background(), code)
		require.NoError(t, err)
		introspectAccessToken(t, conf, token, subject)

		// Reset UI to check for skip values
		testhelpers.NewLoginConsentUI(t, reg.Config(),
			acceptLoginHandler(t, c, subject, func(r *hydra.OAuth2LoginRequest) *hydra.AcceptOAuth2LoginRequest {
				require.True(t, r.Skip)
				require.EqualValues(t, subject, r.Subject)
				return nil
			}),
			acceptConsentHandler(t, c, subject, func(r *hydra.OAuth2ConsentRequest) {
				require.True(t, *r.Skip)
				require.EqualValues(t, subject, *r.Subject)
			}),
		)

		t.Run("followup=checks if authenticatedAt/requestedAt is properly forwarded across the lifecycle by checking if prompt=none works", func(t *testing.T) {
			// In order to check if authenticatedAt/requestedAt works, we'll sleep first in order to ensure that authenticatedAt is in the past
			// if handled correctly.
			time.Sleep(time.Second + time.Nanosecond)

			code, _ := getAuthorizeCode(t, conf, oc,
				oauth2.SetAuthURLParam("nonce", nonce),
				oauth2.SetAuthURLParam("prompt", "none"),
				oauth2.SetAuthURLParam("max_age", "60"),
			)
			require.NotEmpty(t, code)
			token, err := conf.Exchange(context.Background(), code)
			require.NoError(t, err)
			original := introspectAccessToken(t, conf, token, subject)

			t.Run("followup=run the flow three more times", func(t *testing.T) {
				for i := 0; i < 3; i++ {
					t.Run(fmt.Sprintf("run=%d", i), func(t *testing.T) {
						code, _ := getAuthorizeCode(t, conf, oc,
							oauth2.SetAuthURLParam("nonce", nonce),
							oauth2.SetAuthURLParam("prompt", "none"),
							oauth2.SetAuthURLParam("max_age", "60"),
						)
						require.NotEmpty(t, code)
						token, err := conf.Exchange(context.Background(), code)
						require.NoError(t, err)
						followup := introspectAccessToken(t, conf, token, subject)
						assert.Equal(t, original.Get("auth_time").Int(), followup.Get("auth_time").Int())
					})
				}
			})

			t.Run("followup=fails when max age is reached and prompt is none", func(t *testing.T) {
				code, _ := getAuthorizeCode(t, conf, oc,
					oauth2.SetAuthURLParam("nonce", nonce),
					oauth2.SetAuthURLParam("prompt", "none"),
					oauth2.SetAuthURLParam("max_age", "1"),
				)
				require.Empty(t, code)
			})

			t.Run("followup=passes and resets skip when prompt=login", func(t *testing.T) {
				testhelpers.NewLoginConsentUI(t, reg.Config(),
					acceptLoginHandler(t, c, subject, func(r *hydra.OAuth2LoginRequest) *hydra.AcceptOAuth2LoginRequest {
						require.False(t, r.Skip)
						require.Empty(t, r.Subject)
						return nil
					}),
					acceptConsentHandler(t, c, subject, func(r *hydra.OAuth2ConsentRequest) {
						require.True(t, *r.Skip)
						require.EqualValues(t, subject, *r.Subject)
					}),
				)
				code, _ := getAuthorizeCode(t, conf, oc,
					oauth2.SetAuthURLParam("nonce", nonce),
					oauth2.SetAuthURLParam("prompt", "login"),
					oauth2.SetAuthURLParam("max_age", "1"),
				)
				require.NotEmpty(t, code)
				token, err := conf.Exchange(context.Background(), code)
				require.NoError(t, err)
				introspectAccessToken(t, conf, token, subject)
				assertIDToken(t, token, conf, subject, nonce, time.Now().Add(reg.Config().GetIDTokenLifespan(ctx)))
			})
		})
	})

	t.Run("case=should fail if prompt=none but no auth session given", func(t *testing.T) {
		c, conf := newOAuth2Client(t, testhelpers.NewCallbackURL(t, "callback", testhelpers.HTTPServerNotImplementedHandler))
		testhelpers.NewLoginConsentUI(t, reg.Config(),
			acceptLoginHandler(t, c, subject, nil),
			acceptConsentHandler(t, c, subject, nil),
		)

		oc := testhelpers.NewEmptyJarClient(t)
		code, _ := getAuthorizeCode(t, conf, oc,
			oauth2.SetAuthURLParam("prompt", "none"),
		)
		require.Empty(t, code)
	})

	t.Run("case=requires re-authentication when id_token_hint is set to a user 'patrik-neu' but the session is 'aeneas-rekkas' and then fails because the user id from the log in endpoint is 'aeneas-rekkas'", func(t *testing.T) {
		c, conf := newOAuth2Client(t, testhelpers.NewCallbackURL(t, "callback", testhelpers.HTTPServerNotImplementedHandler))
		testhelpers.NewLoginConsentUI(t, reg.Config(),
			acceptLoginHandler(t, c, subject, func(r *hydra.OAuth2LoginRequest) *hydra.AcceptOAuth2LoginRequest {
				require.False(t, r.Skip)
				require.Empty(t, r.Subject)
				return nil
			}),
			acceptConsentHandler(t, c, subject, nil),
		)

		oc := testhelpers.NewEmptyJarClient(t)

		// Create login session for aeneas-rekkas
		code, _ := getAuthorizeCode(t, conf, oc)
		require.NotEmpty(t, code)

		// Perform authentication for aeneas-rekkas which fails because id_token_hint is patrik-neu
		code, _ = getAuthorizeCode(t, conf, oc,
			oauth2.SetAuthURLParam("id_token_hint", testhelpers.NewIDToken(t, reg, "patrik-neu")),
		)
		require.Empty(t, code)
	})

	t.Run("case=should not cause issues if max_age is very low and consent takes a long time", func(t *testing.T) {
		c, conf := newOAuth2Client(t, testhelpers.NewCallbackURL(t, "callback", testhelpers.HTTPServerNotImplementedHandler))
		testhelpers.NewLoginConsentUI(t, reg.Config(),
			acceptLoginHandler(t, c, subject, func(r *hydra.OAuth2LoginRequest) *hydra.AcceptOAuth2LoginRequest {
				time.Sleep(time.Second * 2)
				return nil
			}),
			acceptConsentHandler(t, c, subject, nil),
		)

		code, _ := getAuthorizeCode(t, conf, nil)
		require.NotEmpty(t, code)
	})

	t.Run("case=ensure consistent claims returned for userinfo", func(t *testing.T) {
		c, conf := newOAuth2Client(t, testhelpers.NewCallbackURL(t, "callback", testhelpers.HTTPServerNotImplementedHandler))
		testhelpers.NewLoginConsentUI(t, reg.Config(),
			acceptLoginHandler(t, c, subject, nil),
			acceptConsentHandler(t, c, subject, nil),
		)

		code, _ := getAuthorizeCode(t, conf, nil)
		require.NotEmpty(t, code)

		token, err := conf.Exchange(context.Background(), code)
		require.NoError(t, err)

		idClaims := assertIDToken(t, token, conf, subject, "", time.Now().Add(reg.Config().GetIDTokenLifespan(ctx)))

		time.Sleep(time.Second)
		uiClaims := testhelpers.Userinfo(t, token, publicTS)

		for _, f := range []string{
			"sub",
			"iss",
			"aud",
			"bar",
			"auth_time",
		} {
			assert.NotEmpty(t, uiClaims.Get(f).Raw, "%s: %s", f, uiClaims)
			assert.EqualValues(t, idClaims.Get(f).Raw, uiClaims.Get(f).Raw, "%s\nuserinfo: %s\nidtoken: %s", f, uiClaims, idClaims)
		}

		for _, f := range []string{
			"at_hash",
			"c_hash",
			"nonce",
			"sid",
			"jti",
		} {
			assert.Empty(t, uiClaims.Get(f).Raw, "%s: %s", f, uiClaims)
		}
	})
}

// TestAuthCodeWithMockStrategy runs the authorization_code flow against various ConsentStrategy scenarios.
// For that purpose, the consent strategy is mocked so all scenarios can be applied properly. This test suite checks:
//
// - [x] should pass request if strategy passes
// - [x] should fail because prompt=none and max_age > auth_time
// - [x] should pass because prompt=none and max_age < auth_time
// - [x] should fail because prompt=none but auth_time suggests recent authentication
// - [x] should fail because consent strategy fails
// - [x] should pass with prompt=login when authentication time is recent
// - [x] should fail with prompt=login when authentication time is in the past
func TestAuthCodeWithMockStrategy(t *testing.T) {
	ctx := context.Background()
	for _, strat := range []struct{ d string }{{d: "opaque"}, {d: "jwt"}} {
		t.Run("strategy="+strat.d, func(t *testing.T) {
			conf := internal.NewConfigurationWithDefaults()
			conf.MustSet(ctx, config.KeyAccessTokenLifespan, time.Second*2)
			conf.MustSet(ctx, config.KeyScopeStrategy, "DEPRECATED_HIERARCHICAL_SCOPE_STRATEGY")
			conf.MustSet(ctx, config.KeyAccessTokenStrategy, strat.d)
			reg := internal.NewRegistryMemory(t, conf, &contextx.Default{})
			internal.MustEnsureRegistryKeys(reg, x.OpenIDConnectKeyName)
			internal.MustEnsureRegistryKeys(reg, x.OAuth2JWTKeyName)

			consentStrategy := &consentMock{}
			router := x.NewRouterPublic()
			ts := httptest.NewServer(router)
			defer ts.Close()

			reg.WithConsentStrategy(consentStrategy)
			handler := reg.OAuth2Handler()
			handler.SetRoutes(httprouterx.NewRouterAdminWithPrefixAndRouter(router.Router, "/admin", conf.AdminURL), router, func(h http.Handler) http.Handler {
				return h
			})

			var callbackHandler *httprouter.Handle
			router.GET("/callback", func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
				(*callbackHandler)(w, r, ps)
			})
			var mutex sync.Mutex

			require.NoError(t, reg.ClientManager().CreateClient(context.TODO(), &hc.Client{
				LegacyClientID: "app-client",
				Secret:         "secret",
				RedirectURIs:   []string{ts.URL + "/callback"},
				ResponseTypes:  []string{"id_token", "code", "token"},
				GrantTypes:     []string{"implicit", "refresh_token", "authorization_code", "password", "client_credentials"},
				Scope:          "hydra.* offline openid",
			}))

			oauthConfig := &oauth2.Config{
				ClientID:     "app-client",
				ClientSecret: "secret",
				Endpoint: oauth2.Endpoint{
					AuthURL:  ts.URL + "/oauth2/auth",
					TokenURL: ts.URL + "/oauth2/token",
				},
				RedirectURL: ts.URL + "/callback",
				Scopes:      []string{"hydra.*", "offline", "openid"},
			}

			var code string
			for k, tc := range []struct {
				cj                        http.CookieJar
				d                         string
				cb                        func(t *testing.T) httprouter.Handle
				authURL                   string
				shouldPassConsentStrategy bool
				expectOAuthAuthError      bool
				expectOAuthTokenError     bool
				checkExpiry               bool
				authTime                  time.Time
				requestTime               time.Time
				assertAccessToken         func(*testing.T, string)
			}{
				{
					d:                         "should pass request if strategy passes",
					authURL:                   oauthConfig.AuthCodeURL("some-foo-state"),
					shouldPassConsentStrategy: true,
					checkExpiry:               true,
					cb: func(t *testing.T) httprouter.Handle {
						return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
							code = r.URL.Query().Get("code")
							require.NotEmpty(t, code)
							w.Write([]byte(r.URL.Query().Get("code")))
						}
					},
					assertAccessToken: func(t *testing.T, token string) {
						if strat.d != "jwt" {
							return
						}

						body, err := x.DecodeSegment(strings.Split(token, ".")[1])
						require.NoError(t, err)

						data := map[string]interface{}{}
						require.NoError(t, json.Unmarshal(body, &data))

						assert.EqualValues(t, "app-client", data["client_id"])
						assert.EqualValues(t, "foo", data["sub"])
						assert.NotEmpty(t, data["iss"])
						assert.NotEmpty(t, data["jti"])
						assert.NotEmpty(t, data["exp"])
						assert.NotEmpty(t, data["iat"])
						assert.NotEmpty(t, data["nbf"])
						assert.EqualValues(t, data["nbf"], data["iat"])
						assert.EqualValues(t, []interface{}{"offline", "openid", "hydra.*"}, data["scp"])
					},
				},
				{
					d:                         "should fail because prompt=none and max_age > auth_time",
					authURL:                   oauthConfig.AuthCodeURL("some-foo-state") + "&prompt=none&max_age=1",
					authTime:                  time.Now().UTC().Add(-time.Minute),
					requestTime:               time.Now().UTC(),
					shouldPassConsentStrategy: true,
					cb: func(t *testing.T) httprouter.Handle {
						return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
							code = r.URL.Query().Get("code")
							err := r.URL.Query().Get("error")
							require.Empty(t, code)
							require.EqualValues(t, fosite.ErrLoginRequired.Error(), err)
						}
					},
					expectOAuthAuthError: true,
				},
				{
					d:                         "should pass because prompt=none and max_age is less than auth_time",
					authURL:                   oauthConfig.AuthCodeURL("some-foo-state") + "&prompt=none&max_age=3600",
					authTime:                  time.Now().UTC().Add(-time.Minute),
					requestTime:               time.Now().UTC(),
					shouldPassConsentStrategy: true,
					cb: func(t *testing.T) httprouter.Handle {
						return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
							code = r.URL.Query().Get("code")
							require.NotEmpty(t, code)
							w.Write([]byte(r.URL.Query().Get("code")))
						}
					},
				},
				{
					d:                         "should fail because prompt=none but auth_time suggests recent authentication",
					authURL:                   oauthConfig.AuthCodeURL("some-foo-state") + "&prompt=none",
					authTime:                  time.Now().UTC().Add(-time.Minute),
					requestTime:               time.Now().UTC().Add(-time.Hour),
					shouldPassConsentStrategy: true,
					cb: func(t *testing.T) httprouter.Handle {
						return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
							code = r.URL.Query().Get("code")
							err := r.URL.Query().Get("error")
							require.Empty(t, code)
							require.EqualValues(t, fosite.ErrLoginRequired.Error(), err)
						}
					},
					expectOAuthAuthError: true,
				},
				{
					d:                         "should fail because consent strategy fails",
					authURL:                   oauthConfig.AuthCodeURL("some-foo-state"),
					expectOAuthAuthError:      true,
					shouldPassConsentStrategy: false,
					cb: func(t *testing.T) httprouter.Handle {
						return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
							require.Empty(t, r.URL.Query().Get("code"))
							assert.Equal(t, fosite.ErrRequestForbidden.Error(), r.URL.Query().Get("error"))
						}
					},
				},
				{
					d:                         "should pass with prompt=login when authentication time is recent",
					authURL:                   oauthConfig.AuthCodeURL("some-foo-state") + "&prompt=login",
					authTime:                  time.Now().UTC().Add(-time.Second),
					requestTime:               time.Now().UTC().Add(-time.Minute),
					shouldPassConsentStrategy: true,
					cb: func(t *testing.T) httprouter.Handle {
						return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
							code = r.URL.Query().Get("code")
							require.NotEmpty(t, code)
							w.Write([]byte(r.URL.Query().Get("code")))
						}
					},
				},
				{
					d:                         "should fail with prompt=login when authentication time is in the past",
					authURL:                   oauthConfig.AuthCodeURL("some-foo-state") + "&prompt=login",
					authTime:                  time.Now().UTC().Add(-time.Minute),
					requestTime:               time.Now().UTC(),
					expectOAuthAuthError:      true,
					shouldPassConsentStrategy: true,
					cb: func(t *testing.T) httprouter.Handle {
						return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
							code = r.URL.Query().Get("code")
							require.Empty(t, code)
							assert.Equal(t, fosite.ErrLoginRequired.Error(), r.URL.Query().Get("error"))
						}
					},
				},
			} {
				t.Run(fmt.Sprintf("case=%d/description=%s", k, tc.d), func(t *testing.T) {
					mutex.Lock()
					defer mutex.Unlock()
					if tc.cb == nil {
						tc.cb = noopHandler
					}

					consentStrategy.deny = !tc.shouldPassConsentStrategy
					consentStrategy.authTime = tc.authTime
					consentStrategy.requestTime = tc.requestTime

					cb := tc.cb(t)
					callbackHandler = &cb

					req, err := http.NewRequest("GET", tc.authURL, nil)
					require.NoError(t, err)

					if tc.cj == nil {
						tc.cj = testhelpers.NewEmptyCookieJar(t)
					}

					resp, err := (&http.Client{Jar: tc.cj}).Do(req)
					require.NoError(t, err, tc.authURL, ts.URL)
					defer resp.Body.Close()

					if tc.expectOAuthAuthError {
						require.Empty(t, code)
						return
					}

					require.NotEmpty(t, code)

					token, err := oauthConfig.Exchange(oauth2.NoContext, code)
					if tc.expectOAuthTokenError {
						require.Error(t, err)
						return
					}

					require.NoError(t, err, code)
					if tc.assertAccessToken != nil {
						tc.assertAccessToken(t, token.AccessToken)
					}

					t.Run("case=userinfo", func(t *testing.T) {
						var makeRequest = func(req *http.Request) *http.Response {
							resp, err = http.DefaultClient.Do(req)
							require.NoError(t, err)
							return resp
						}

						var testSuccess = func(response *http.Response) {
							defer resp.Body.Close()

							require.Equal(t, http.StatusOK, resp.StatusCode)

							var claims map[string]interface{}
							require.NoError(t, json.NewDecoder(resp.Body).Decode(&claims))
							assert.Equal(t, "foo", claims["sub"])
						}

						req, err = http.NewRequest("GET", ts.URL+"/userinfo", nil)
						req.Header.Add("Authorization", "bearer "+token.AccessToken)
						testSuccess(makeRequest(req))

						req, err = http.NewRequest("POST", ts.URL+"/userinfo", nil)
						req.Header.Add("Authorization", "bearer "+token.AccessToken)
						testSuccess(makeRequest(req))

						req, err = http.NewRequest("POST", ts.URL+"/userinfo", bytes.NewBuffer([]byte("access_token="+token.AccessToken)))
						req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
						testSuccess(makeRequest(req))

						req, err = http.NewRequest("GET", ts.URL+"/userinfo", nil)
						req.Header.Add("Authorization", "bearer asdfg")
						resp := makeRequest(req)
						require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
					})

					res, err := testRefresh(t, token, ts.URL, tc.checkExpiry)
					require.NoError(t, err)
					assert.Equal(t, http.StatusOK, res.StatusCode)

					body, err := io.ReadAll(res.Body)
					require.NoError(t, err)

					var refreshedToken oauth2.Token
					require.NoError(t, json.Unmarshal(body, &refreshedToken))

					if tc.assertAccessToken != nil {
						tc.assertAccessToken(t, refreshedToken.AccessToken)
					}

					t.Run("the tokens should be different", func(t *testing.T) {
						if strat.d != "jwt" {
							t.Skip()
						}

						body, err := x.DecodeSegment(strings.Split(token.AccessToken, ".")[1])
						require.NoError(t, err)

						origPayload := map[string]interface{}{}
						require.NoError(t, json.Unmarshal(body, &origPayload))

						body, err = x.DecodeSegment(strings.Split(refreshedToken.AccessToken, ".")[1])
						require.NoError(t, err)

						refreshedPayload := map[string]interface{}{}
						require.NoError(t, json.Unmarshal(body, &refreshedPayload))

						if tc.checkExpiry {
							assert.NotEqual(t, refreshedPayload["exp"], origPayload["exp"])
							assert.NotEqual(t, refreshedPayload["iat"], origPayload["iat"])
							assert.NotEqual(t, refreshedPayload["nbf"], origPayload["nbf"])
						}
						assert.NotEqual(t, refreshedPayload["jti"], origPayload["jti"])
						assert.Equal(t, refreshedPayload["client_id"], origPayload["client_id"])
					})

					require.NotEqual(t, token.AccessToken, refreshedToken.AccessToken)

					t.Run("old token should no longer be usable", func(t *testing.T) {
						req, err := http.NewRequest("GET", ts.URL+"/userinfo", nil)
						require.NoError(t, err)
						req.Header.Add("Authorization", "bearer "+token.AccessToken)
						res, err := http.DefaultClient.Do(req)
						require.NoError(t, err)
						assert.EqualValues(t, http.StatusUnauthorized, res.StatusCode)
					})

					t.Run("refreshing new refresh token should work", func(t *testing.T) {
						res, err := testRefresh(t, &refreshedToken, ts.URL, false)
						require.NoError(t, err)
						assert.Equal(t, http.StatusOK, res.StatusCode)

						body, err := io.ReadAll(res.Body)
						require.NoError(t, err)
						require.NoError(t, json.Unmarshal(body, &refreshedToken))
					})

					t.Run("should call refresh token hook if configured", func(t *testing.T) {
						hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							assert.Equal(t, r.Header.Get("Content-Type"), "application/json; charset=UTF-8")

							expectedGrantedScopes := []string{"openid", "offline", "hydra.*"}
							expectedSubject := "foo"

							var hookReq hydraoauth2.RefreshTokenHookRequest
							require.NoError(t, json.NewDecoder(r.Body).Decode(&hookReq))
							require.Equal(t, hookReq.Subject, expectedSubject)
							require.ElementsMatch(t, hookReq.GrantedScopes, expectedGrantedScopes)
							require.ElementsMatch(t, hookReq.GrantedAudience, []string{})
							require.Equal(t, hookReq.ClientID, oauthConfig.ClientID)
							require.NotEmpty(t, hookReq.Session)
							require.Equal(t, hookReq.Session.Subject, expectedSubject)
							require.Equal(t, hookReq.Session.ClientID, oauthConfig.ClientID)
							require.Equal(t, hookReq.Session.Extra, map[string]interface{}{})
							require.NotEmpty(t, hookReq.Requester)
							require.Equal(t, hookReq.Requester.ClientID, oauthConfig.ClientID)
							require.ElementsMatch(t, hookReq.Requester.GrantedScopes, expectedGrantedScopes)

							except := []string{
								"session.kid",
								"session.id_token.expires_at",
								"session.id_token.headers.extra.kid",
								"session.id_token.id_token_claims.iat",
								"session.id_token.id_token_claims.exp",
								"session.id_token.id_token_claims.rat",
								"session.id_token.id_token_claims.auth_time",
							}
							snapshotx.SnapshotTExcept(t, hookReq, except)

							claims := map[string]interface{}{
								"hooked": true,
							}

							hookResp := hydraoauth2.RefreshTokenHookResponse{
								Session: consent.AcceptOAuth2ConsentRequestSession{
									AccessToken: claims,
									IDToken:     claims,
								},
							}

							w.WriteHeader(http.StatusOK)
							require.NoError(t, json.NewEncoder(w).Encode(&hookResp))
						}))
						defer hs.Close()

						conf.MustSet(ctx, config.KeyRefreshTokenHookURL, hs.URL)
						defer conf.MustSet(ctx, config.KeyRefreshTokenHookURL, nil)

						res, err := testRefresh(t, &refreshedToken, ts.URL, false)
						require.NoError(t, err)
						assert.Equal(t, http.StatusOK, res.StatusCode)

						body, err := io.ReadAll(res.Body)
						require.NoError(t, err)
						require.NoError(t, json.Unmarshal(body, &refreshedToken))

						accessTokenClaims := testhelpers.IntrospectToken(t, oauthConfig, refreshedToken.AccessToken, ts)
						require.True(t, accessTokenClaims.Get("ext.hooked").Bool())

						idTokenBody, err := x.DecodeSegment(
							strings.Split(
								gjson.GetBytes(body, "id_token").String(),
								".",
							)[1],
						)
						require.NoError(t, err)

						require.True(t, gjson.GetBytes(idTokenBody, "hooked").Bool())
					})

					t.Run("should not override session data if token refresh hook returns no content", func(t *testing.T) {
						hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							w.WriteHeader(http.StatusNoContent)
						}))
						defer hs.Close()

						conf.MustSet(ctx, config.KeyRefreshTokenHookURL, hs.URL)
						defer conf.MustSet(ctx, config.KeyRefreshTokenHookURL, nil)

						origAccessTokenClaims := testhelpers.IntrospectToken(t, oauthConfig, refreshedToken.AccessToken, ts)

						res, err := testRefresh(t, &refreshedToken, ts.URL, false)
						require.NoError(t, err)
						assert.Equal(t, http.StatusOK, res.StatusCode)

						body, err = io.ReadAll(res.Body)
						require.NoError(t, err)

						require.NoError(t, json.Unmarshal(body, &refreshedToken))

						refreshedAccessTokenClaims := testhelpers.IntrospectToken(t, oauthConfig, refreshedToken.AccessToken, ts)
						assertx.EqualAsJSONExcept(t, json.RawMessage(origAccessTokenClaims.Raw), json.RawMessage(refreshedAccessTokenClaims.Raw), []string{"exp", "iat", "nbf"})
					})

					t.Run("should fail token refresh with `server_error` if hook fails", func(t *testing.T) {
						hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							w.WriteHeader(http.StatusInternalServerError)
						}))
						defer hs.Close()

						conf.MustSet(ctx, config.KeyRefreshTokenHookURL, hs.URL)
						defer conf.MustSet(ctx, config.KeyRefreshTokenHookURL, nil)

						res, err := testRefresh(t, &refreshedToken, ts.URL, false)
						require.NoError(t, err)
						assert.Equal(t, http.StatusInternalServerError, res.StatusCode)

						var errBody fosite.RFC6749ErrorJson
						require.NoError(t, json.NewDecoder(res.Body).Decode(&errBody))
						require.Equal(t, fosite.ErrServerError.Error(), errBody.Name)
						require.Equal(t, "An error occurred while executing the refresh token hook.", errBody.Description)
					})

					t.Run("should fail token refresh with `access_denied` if hook denied the request", func(t *testing.T) {
						hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							w.WriteHeader(http.StatusForbidden)
						}))
						defer hs.Close()

						conf.MustSet(ctx, config.KeyRefreshTokenHookURL, hs.URL)
						defer conf.MustSet(ctx, config.KeyRefreshTokenHookURL, nil)

						res, err := testRefresh(t, &refreshedToken, ts.URL, false)
						require.NoError(t, err)
						assert.Equal(t, http.StatusForbidden, res.StatusCode)

						var errBody fosite.RFC6749ErrorJson
						require.NoError(t, json.NewDecoder(res.Body).Decode(&errBody))
						require.Equal(t, fosite.ErrAccessDenied.Error(), errBody.Name)
						require.Equal(t, "The refresh token hook target responded with an error. Make sure that the request you are making is valid. Maybe the credential or request parameters you are using are limited in scope or otherwise restricted.", errBody.Description)
					})

					t.Run("should fail token refresh with `server_error` if hook response is malformed", func(t *testing.T) {
						hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							w.WriteHeader(http.StatusOK)
						}))
						defer hs.Close()

						conf.MustSet(ctx, config.KeyRefreshTokenHookURL, hs.URL)
						defer conf.MustSet(ctx, config.KeyRefreshTokenHookURL, nil)

						res, err := testRefresh(t, &refreshedToken, ts.URL, false)
						require.NoError(t, err)
						assert.Equal(t, http.StatusInternalServerError, res.StatusCode)

						var errBody fosite.RFC6749ErrorJson
						require.NoError(t, json.NewDecoder(res.Body).Decode(&errBody))
						require.Equal(t, fosite.ErrServerError.Error(), errBody.Name)
						require.Equal(t, "The refresh token hook target responded with an error.", errBody.Description)
					})

					t.Run("refreshing old token should no longer work", func(t *testing.T) {
						res, err := testRefresh(t, token, ts.URL, false)
						require.NoError(t, err)
						assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
					})

					t.Run("attempt to refresh old token should revoke new token", func(t *testing.T) {
						res, err := testRefresh(t, &refreshedToken, ts.URL, false)
						require.NoError(t, err)
						assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
					})

					t.Run("duplicate code exchange fails", func(t *testing.T) {
						token, err := oauthConfig.Exchange(oauth2.NoContext, code)
						require.Error(t, err)
						require.Nil(t, token)
					})

					code = ""
				})
			}
		})
	}
}

func testRefresh(t *testing.T, token *oauth2.Token, u string, sleep bool) (*http.Response, error) {
	if sleep {
		time.Sleep(time.Millisecond * 1001)
	}

	oauthClientConfig := &clientcredentials.Config{
		ClientID:     "app-client",
		ClientSecret: "secret",
		TokenURL:     u + "/oauth2/token",
		Scopes:       []string{"foobar"},
	}

	req, err := http.NewRequest("POST", oauthClientConfig.TokenURL, strings.NewReader(url.Values{
		"grant_type":    []string{"refresh_token"},
		"refresh_token": []string{token.RefreshToken},
	}.Encode()))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(oauthClientConfig.ClientID, oauthClientConfig.ClientSecret)

	return http.DefaultClient.Do(req)
}
