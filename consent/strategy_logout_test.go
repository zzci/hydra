package consent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ory/x/pointerx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	jwtgo "github.com/ory/fosite/token/jwt"

	hydra "github.com/ory/hydra-client-go"
	"github.com/ory/hydra/client"
	"github.com/ory/hydra/driver/config"
	"github.com/ory/hydra/internal"
	"github.com/ory/hydra/internal/testhelpers"
	"github.com/ory/x/contextx"
	"github.com/ory/x/ioutilx"
)

func TestLogoutFlows(t *testing.T) {
	ctx := context.Background()
	reg := internal.NewMockedRegistry(t, &contextx.Default{})
	reg.Config().MustSet(ctx, config.KeyAccessTokenStrategy, "opaque")
	reg.Config().MustSet(ctx, config.KeyConsentRequestMaxAge, time.Hour)

	defaultRedirectedMessage := "redirected to default server"
	postLogoutCallback := func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		_, _ = fmt.Fprintf(w, "%s%s%s", defaultRedirectedMessage, r.Form.Get("state"), strings.TrimLeft(r.URL.Path, "/"))
	}
	defaultLogoutURL := testhelpers.NewCallbackURL(t, "logged-out", postLogoutCallback)
	customPostLogoutURL := testhelpers.NewCallbackURL(t, "logged-out/custom", postLogoutCallback)
	reg.Config().MustSet(ctx, config.KeyLogoutRedirectURL, defaultLogoutURL)

	publicTS, adminTS := testhelpers.NewOAuth2Server(ctx, t, reg)

	adminApi := hydra.NewAPIClient(hydra.NewConfiguration())
	adminApi.GetConfig().Servers = hydra.ServerConfigurations{{URL: adminTS.URL}}

	createBrowserWithSession := func(t *testing.T, c *client.Client) *http.Client {
		hc := testhelpers.NewEmptyJarClient(t)
		makeOAuth2Request(t, reg, hc, c, url.Values{})
		return hc
	}

	createSampleClient := func(t *testing.T) *client.Client {
		return createClient(t, reg, &client.Client{
			RedirectURIs:           []string{testhelpers.NewCallbackURL(t, "callback", testhelpers.HTTPServerNotImplementedHandler)},
			PostLogoutRedirectURIs: []string{customPostLogoutURL}})
	}

	createClientWithBackchannelLogout := func(t *testing.T, wg *sync.WaitGroup, cb func(t *testing.T, logoutToken gjson.Result)) *client.Client {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer wg.Done()

			require.NoError(t, r.ParseForm())
			lt := r.PostFormValue("logout_token")
			assert.NotEmpty(t, lt)
			token, err := reg.OpenIDJWTStrategy().Decode(r.Context(), lt)
			require.NoError(t, err)

			var b bytes.Buffer
			require.NoError(t, json.NewEncoder(&b).Encode(token.Claims))
			cb(t, gjson.Parse(b.String()))
		}))
		t.Cleanup(server.Close)

		return createClient(t, reg, &client.Client{
			BackChannelLogoutURI:   server.URL,
			RedirectURIs:           []string{testhelpers.NewCallbackURL(t, "callback", testhelpers.HTTPServerNotImplementedHandler)},
			PostLogoutRedirectURIs: []string{customPostLogoutURL}})
	}

	makeLogoutRequest := func(t *testing.T, hc *http.Client, method string, values url.Values) (body string, resp *http.Response) {
		var err error
		if method == http.MethodGet {
			resp, err = hc.Get(publicTS.URL + "/oauth2/sessions/logout?" + values.Encode())
		} else if method == http.MethodPost {
			resp, err = hc.PostForm(publicTS.URL+"/oauth2/sessions/logout", values)
		}
		require.NoError(t, err)
		defer resp.Body.Close()
		return string(ioutilx.MustReadAll(resp.Body)), resp
	}

	logoutAndExpectErrorPage := func(t *testing.T, browser *http.Client, method string, values url.Values, expectedErrorMessage string) {
		body, res := makeLogoutRequest(t, browser, method, values)
		assert.EqualValues(t, http.StatusInternalServerError, res.StatusCode)
		assert.Contains(t, body, expectedErrorMessage)
	}

	testExpectErrorPage := func(browser *http.Client, method string, values url.Values, expectedErrorMessage string) func(t *testing.T) {
		return func(t *testing.T) {
			logoutAndExpectErrorPage(t, browser, method, values, expectedErrorMessage)
		}
	}

	logoutAndExpectPostLogoutPage := func(t *testing.T, browser *http.Client, method string, values url.Values, expectedMessage string) {
		body, res := makeLogoutRequest(t, browser, method, values)
		assert.EqualValues(t, http.StatusOK, res.StatusCode)
		assert.Contains(t, body, expectedMessage)
	}

	testExpectPostLogoutPage := func(browser *http.Client, method string, values url.Values, expectedMessage string) func(t *testing.T) {
		return func(t *testing.T) {
			logoutAndExpectPostLogoutPage(t, browser, method, values, expectedMessage)
		}
	}

	browserWithoutSession := new(http.Client)

	newWg := func(add int) *sync.WaitGroup {
		var wg sync.WaitGroup
		wg.Add(add)
		return &wg
	}

	checkAndAcceptLogout := func(t *testing.T, wg *sync.WaitGroup, cb func(*testing.T, *hydra.OAuth2LogoutRequest, error)) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if wg != nil {
				defer wg.Done()
			}

			res, _, err := adminApi.OAuth2Api.GetOAuth2LogoutRequest(ctx).LogoutChallenge(r.URL.Query().Get("logout_challenge")).Execute()
			if cb != nil {
				cb(t, res, err)
			}

			v, _, err := adminApi.OAuth2Api.AcceptOAuth2LogoutRequest(ctx).LogoutChallenge(r.URL.Query().Get("logout_challenge")).Execute()
			require.NoError(t, err)
			require.NotEmpty(t, v.RedirectTo)
			http.Redirect(w, r, v.RedirectTo, http.StatusFound)
		}))

		t.Cleanup(server.Close)

		reg.Config().MustSet(ctx, config.KeyLogoutURL, server.URL)
	}

	acceptLoginAsAndWatchSid := func(t *testing.T, subject string, sid chan string) {
		testhelpers.NewLoginConsentUI(t, reg.Config(),
			checkAndAcceptLoginHandler(t, adminApi, subject, func(t *testing.T, res *hydra.OAuth2LoginRequest, err error) hydra.AcceptOAuth2LoginRequest {
				require.NoError(t, err)
				//res.Payload.SessionID
				return hydra.AcceptOAuth2LoginRequest{Remember: pointerx.Bool(true)}
			}),
			checkAndAcceptConsentHandler(t, adminApi, func(t *testing.T, res *hydra.OAuth2ConsentRequest, err error) hydra.AcceptOAuth2ConsentRequest {
				require.NoError(t, err)
				if sid != nil {
					go func() {
						sid <- *res.LoginSessionId
					}()
				}
				return hydra.AcceptOAuth2ConsentRequest{Remember: pointerx.Bool(true)}
			}))

	}

	acceptLoginAs := func(t *testing.T, subject string) {
		acceptLoginAsAndWatchSid(t, subject, nil)
	}

	subject := "aeneas-rekkas"

	t.Run("case=should ignore / redirect non-rp initiated logout if no session exists", func(t *testing.T) {
		t.Run("method=get", testExpectPostLogoutPage(browserWithoutSession, http.MethodGet, url.Values{}, defaultRedirectedMessage))
		t.Run("method=post", testExpectPostLogoutPage(browserWithoutSession, http.MethodPost, url.Values{}, defaultRedirectedMessage))
	})

	t.Run("case=should fail if non-rp initiated logout is initiated with state (indicating rp-flow)", func(t *testing.T) {
		expectedMessage := "Logout failed because query parameter state is set but id_token_hint is missing"
		values := url.Values{"state": {"foobar"}}
		t.Run("method=get", testExpectErrorPage(browserWithoutSession, http.MethodGet, values, expectedMessage))
		t.Run("method=post", testExpectErrorPage(browserWithoutSession, http.MethodPost, values, expectedMessage))
	})

	t.Run("case=should fail if non-rp initiated logout is initiated with post_logout_redirect_uri (indicating rp-flow)", func(t *testing.T) {
		expectedMessage := "Logout failed because query parameter post_logout_redirect_uri is set but id_token_hint is missing"
		values := url.Values{"post_logout_redirect_uri": {"foobar"}}
		t.Run("method=get", testExpectErrorPage(browserWithoutSession, http.MethodGet, values, expectedMessage))
		t.Run("method=post", testExpectErrorPage(browserWithoutSession, http.MethodPost, values, expectedMessage))
	})

	t.Run("case=should ignore / redirect non-rp initiated logout if a session cookie exists but the session itself is no longer active / invalid", func(t *testing.T) {
		browser := &http.Client{Jar: newAuthCookieJar(t, reg, publicTS.URL, "i-do-not-exist")}
		t.Run("method=get", testExpectPostLogoutPage(browser, http.MethodGet, url.Values{}, defaultRedirectedMessage))
		t.Run("method=post", testExpectPostLogoutPage(browser, http.MethodPost, url.Values{}, defaultRedirectedMessage))
	})

	t.Run("case=should redirect to logout provider if session exists and it's not rp-flow", func(t *testing.T) {
		acceptLoginAs(t, subject)

		wg := newWg(2)
		checkAndAcceptLogout(t, wg, func(t *testing.T, res *hydra.OAuth2LogoutRequest, err error) {
			require.NoError(t, err)
			assert.EqualValues(t, subject, *res.Subject)
			assert.NotEmpty(t, subject, res.Sid)
		})

		t.Run("method=get", testExpectPostLogoutPage(createBrowserWithSession(t, createSampleClient(t)), http.MethodGet, url.Values{}, defaultRedirectedMessage))

		t.Run("method=post", testExpectPostLogoutPage(createBrowserWithSession(t, createSampleClient(t)), http.MethodPost, url.Values{}, defaultRedirectedMessage))

		wg.Wait() // we want to ensure that logout ui was called!
	})

	t.Run("case=should redirect to post logout url because logout was already done before", func(t *testing.T) {
		// Formerly: should redirect to logout provider because the session has been removed previously
		acceptLoginAs(t, subject)
		browser := createBrowserWithSession(t, createSampleClient(t))

		// run once to invalidate session
		wg := newWg(1)
		checkAndAcceptLogout(t, wg, nil)
		logoutAndExpectPostLogoutPage(t, browser, http.MethodGet, url.Values{}, defaultRedirectedMessage)

		t.Run("method=get", testExpectPostLogoutPage(browser, http.MethodGet, url.Values{}, defaultRedirectedMessage))
		t.Run("method=post", testExpectPostLogoutPage(browser, http.MethodPost, url.Values{}, defaultRedirectedMessage))

		wg.Wait() // we want to ensure that logout ui was called exactly once
	})

	t.Run("case=should execute backchannel logout if issued without rp-involvement", func(t *testing.T) {
		sid := make(chan string)
		acceptLoginAsAndWatchSid(t, subject, sid)

		logoutWg := newWg(2)
		checkAndAcceptLogout(t, logoutWg, nil)

		backChannelWG := newWg(2)
		c := createClientWithBackchannelLogout(t, backChannelWG, func(t *testing.T, logoutToken gjson.Result) {
			assert.EqualValues(t, <-sid, logoutToken.Get("sid").String(), logoutToken.Raw)
			assert.Empty(t, logoutToken.Get("sub").String(), logoutToken.Raw) // The sub claim should be empty because it doesn't work with forced obfuscation and thus we can't easily recover it.
			assert.Empty(t, logoutToken.Get("nonce").String(), logoutToken.Raw)
		})

		t.Run("method=get", testExpectPostLogoutPage(createBrowserWithSession(t, c), http.MethodGet, url.Values{}, defaultRedirectedMessage))

		t.Run("method=post", testExpectPostLogoutPage(createBrowserWithSession(t, c), http.MethodPost, url.Values{}, defaultRedirectedMessage))

		logoutWg.Wait()      // we want to ensure that logout ui was called!
		backChannelWG.Wait() // we want to ensure that all back channels have been called!
	})

	// Only do GET requests from here on out, POST should be tested enough to ensure that it is working fine already.

	t.Run("case=should fail several flows when id_token_hint is invalid", func(t *testing.T) {
		t.Run("case=should error when rp-flow without valid id token", func(t *testing.T) {
			acceptLoginAs(t, "aeneas-rekkas")
			checkAndAcceptLogout(t, nil, nil)

			expectedMessage := "compact JWS format must have three parts"
			browser := createBrowserWithSession(t, createSampleClient(t))
			values := url.Values{"state": {"1234"}, "post_logout_redirect_uri": {customPostLogoutURL}, "id_token_hint": {"i am not valid"}}
			t.Run("method=get", testExpectErrorPage(browser, http.MethodGet, values, expectedMessage))
			t.Run("method=post", testExpectErrorPage(browser, http.MethodPost, values, expectedMessage))
		})

		for _, tc := range []struct {
			d                  string
			claims             jwtgo.MapClaims
			expectedErrMessage string
		}{
			{
				d: "should fail rp-inititated flow because id token hint is missing issuer",
				claims: jwtgo.MapClaims{
					"iat": time.Now().Add(-time.Hour * 2).Unix(),
				},
				expectedErrMessage: "Logout failed because issuer claim value &#39;&#39; from query parameter id_token_hint does not match with issuer value from configuration",
			},
			{
				d: "should fail rp-inititated flow because id token hint is using wrong issuer",
				claims: jwtgo.MapClaims{
					"iss": "some-issuer",
					"iat": time.Now().Add(-time.Hour * 2).Unix(),
				},
				expectedErrMessage: "Logout failed because issuer claim value &#39;some-issuer&#39; from query parameter id_token_hint does not match with issuer value from configuration",
			},
			{
				d: "should fail rp-inititated flow because iat is in the future",
				claims: jwtgo.MapClaims{
					"iss": reg.Config().IssuerURL(ctx).String(),
					"iat": time.Now().Add(time.Hour * 2).Unix(),
				},
				expectedErrMessage: "Token used before issued",
			},
		} {
			t.Run("case="+tc.d, func(t *testing.T) {

				c := createSampleClient(t)
				sid := make(chan string)
				acceptLoginAsAndWatchSid(t, subject, sid)
				browser := createBrowserWithSession(t, c)

				wg := newWg(1)
				checkAndAcceptLogout(t, wg, nil)
				tc.claims["sub"] = subject
				tc.claims["sid"] = <-sid
				tc.claims["aud"] = c.GetID()
				tc.claims["exp"] = time.Now().Add(-time.Hour).Unix()

				logoutAndExpectErrorPage(t, browser, http.MethodGet, url.Values{
					"state":                    {"1234"},
					"post_logout_redirect_uri": {customPostLogoutURL},
					"id_token_hint":            {testhelpers.NewIDTokenWithClaims(t, reg, tc.claims)},
				}, tc.expectedErrMessage)

				wg.Done()
			})
		}
	})

	t.Run("case=should fail because post-logout url is not registered", func(t *testing.T) {
		c := createSampleClient(t)
		acceptLoginAs(t, subject)

		browser := createBrowserWithSession(t, c)
		values := url.Values{
			"state":                    {"1234"},
			"post_logout_redirect_uri": {"https://this-is-not-a-valid-redirect-url/custom"},
			"id_token_hint": {testhelpers.NewIDTokenWithClaims(t, reg, jwtgo.MapClaims{
				"aud": c.GetID(),
				"iss": reg.Config().IssuerURL(ctx).String(),
				"sub": subject,
				"sid": "logout-session-temp4",
				"exp": time.Now().Add(-time.Hour).Unix(),
				"iat": time.Now().Add(-time.Hour * 2).Unix(),
			})},
		}

		logoutAndExpectErrorPage(t, browser, http.MethodGet, values, "Logout failed because query parameter post_logout_redirect_uri is not a whitelisted as a post_logout_redirect_uri for the client")
	})

	t.Run("case=should pass rp-initiated flows", func(t *testing.T) {
		c := createSampleClient(t)
		run := func(method string, claims jwtgo.MapClaims) func(t *testing.T) {
			return func(t *testing.T) {
				sid := make(chan string)
				acceptLoginAsAndWatchSid(t, subject, sid)

				checkAndAcceptLogout(t, nil, nil)
				browser := createBrowserWithSession(t, c)

				sendClaims := jwtgo.MapClaims{
					"iss": reg.Config().IssuerURL(ctx).String(),
					"aud": c.GetID(),
					"sid": <-sid,
					"sub": subject,
					"exp": time.Now().Add(time.Hour).Unix(),
					"iat": time.Now().Add(-time.Hour).Unix(),
				}

				for k, v := range claims {
					sendClaims[k] = v
				}

				body, res := makeLogoutRequest(t, browser, method, url.Values{
					"state":                    {"1234"},
					"post_logout_redirect_uri": {customPostLogoutURL},
					"id_token_hint":            {testhelpers.NewIDTokenWithClaims(t, reg, sendClaims)},
				})

				assert.EqualValues(t, http.StatusOK, res.StatusCode)
				assert.Contains(t, body, "redirected to default server1234logged-out/custom")
				assert.Contains(t, res.Request.URL.String(), "/logged-out/custom?state=1234")
			}
		}

		t.Run("case=should pass even if expiry is in the past", func(t *testing.T) {
			// formerly: should pass rp-inititated even when expiry is in the past
			claims := jwtgo.MapClaims{"exp": time.Now().Add(-time.Hour).Unix()}
			t.Run("method=GET", run("GET", claims))
			t.Run("method=POST", run("POST", claims))
		})

		t.Run("case=should pass even if audience is an array not a string", func(t *testing.T) {
			// formerly: should pass rp-inititated flow"
			claims := jwtgo.MapClaims{"aud": []string{c.GetID()}}
			t.Run("method=GET", run("GET", claims))
			t.Run("method=POST", run("POST", claims))
		})
	})

	t.Run("case=should pass rp-inititated flow without any action because SID is unknown", func(t *testing.T) {
		c := createSampleClient(t)
		acceptLoginAsAndWatchSid(t, subject, nil)

		checkAndAcceptLogout(t, nil, func(t *testing.T, res *hydra.OAuth2LogoutRequest, err error) {
			t.Fatalf("Logout should not have been called")
		})
		browser := createBrowserWithSession(t, c)

		logoutAndExpectPostLogoutPage(t, browser, "GET", url.Values{
			"state":                    {"1234"},
			"post_logout_redirect_uri": {customPostLogoutURL},
			"id_token_hint": {genIDToken(t, reg, jwtgo.MapClaims{
				"aud": []string{c.GetID()}, // make sure this works with string slices too
				"iss": reg.Config().IssuerURL(ctx).String(),
				"sub": subject,
				"sid": "i-do-not-exist",
				"exp": time.Now().Add(time.Hour).Unix(),
				"iat": time.Now().Add(-time.Hour).Unix(),
			})},
		}, defaultRedirectedMessage+"1234logged-out/custom")
	})

	t.Run("case=should not append a state param if no state was passed to logout server", func(t *testing.T) {
		c := createSampleClient(t)
		sid := make(chan string)
		acceptLoginAsAndWatchSid(t, subject, sid)

		checkAndAcceptLogout(t, nil, nil)
		browser := createBrowserWithSession(t, c)

		body, res := makeLogoutRequest(t, browser, "GET", url.Values{
			"post_logout_redirect_uri": {customPostLogoutURL},
			"id_token_hint": {testhelpers.NewIDTokenWithClaims(t, reg, jwtgo.MapClaims{
				"iss": reg.Config().IssuerURL(ctx).String(),
				"aud": c.GetID(),
				"sid": <-sid,
				"sub": subject,
				"exp": time.Now().Add(time.Hour).Unix(),
				"iat": time.Now().Add(-time.Hour).Unix(),
			})},
		})

		assert.EqualValues(t, http.StatusOK, res.StatusCode)
		assert.Contains(t, body, "redirected to default serverlogged-out/custom")
		assert.Contains(t, res.Request.URL.String(), "/logged-out/custom")
		assert.NotContains(t, res.Request.URL.String(), "state=1234")
	})

	t.Run("case=should return to default post logout because session was revoked in browser context", func(t *testing.T) {
		c := createSampleClient(t)
		sid := make(chan string)
		acceptLoginAsAndWatchSid(t, subject, sid)

		wg := newWg(2)
		checkAndAcceptLogout(t, wg, nil)
		browser := createBrowserWithSession(t, c)

		// Use another browser (without session cookie) to make the logout request:
		otherBrowser := &http.Client{}
		logoutAndExpectPostLogoutPage(t, otherBrowser, "GET", url.Values{
			"post_logout_redirect_uri": {customPostLogoutURL},
			"id_token_hint": {testhelpers.NewIDTokenWithClaims(t, reg, jwtgo.MapClaims{
				"iss": reg.Config().IssuerURL(ctx).String(),
				"aud": c.GetID(),
				"sid": <-sid,
				"sub": subject,
				"exp": time.Now().Add(time.Hour).Unix(),
				"iat": time.Now().Add(-time.Hour).Unix(),
			})},
		}, "redirected to default serverlogged-out/custom") // this means RP-initiated flow worked!

		// Set up login / consent and check if skip is set to false (because logout happened), but use
		// the original login browser which still has the session.
		testhelpers.NewLoginConsentUI(t, reg.Config(),
			checkAndAcceptLoginHandler(t, adminApi, subject, func(t *testing.T, res *hydra.OAuth2LoginRequest, err error) hydra.AcceptOAuth2LoginRequest {
				defer wg.Done()
				require.NoError(t, err)
				assert.False(t, res.Skip)
				return hydra.AcceptOAuth2LoginRequest{Remember: pointerx.Bool(true)}
			}),
			checkAndAcceptConsentHandler(t, adminApi, func(t *testing.T, res *hydra.OAuth2ConsentRequest, err error) hydra.AcceptOAuth2ConsentRequest {
				require.NoError(t, err)
				return hydra.AcceptOAuth2ConsentRequest{Remember: pointerx.Bool(true)}
			}))

		// Make an oauth 2 request to trigger the login check.
		_, res := makeOAuth2Request(t, reg, browser, c, url.Values{})
		assert.EqualValues(t, http.StatusNotImplemented, res.StatusCode)
		assert.NotEmpty(t, res.Request.URL.Query().Get("code"))

		wg.Wait()
	})
}
