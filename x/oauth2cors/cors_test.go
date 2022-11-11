// Copyright © 2022 Ory Corp
// SPDX-License-Identifier: Apache-2.0

package oauth2cors_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ory/hydra/driver"
	"github.com/ory/hydra/x/oauth2cors"
	"github.com/ory/x/contextx"

	"github.com/ory/hydra/x"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ory/fosite"
	"github.com/ory/hydra/client"
	"github.com/ory/hydra/internal"
	"github.com/ory/hydra/oauth2"
)

func TestOAuth2AwareCORSMiddleware(t *testing.T) {
	r := internal.NewRegistryMemory(t, internal.NewConfigurationWithDefaults(), &contextx.Default{})
	token, signature, _ := r.OAuth2HMACStrategy().GenerateAccessToken(nil, nil)
	for k, tc := range []struct {
		prep         func(*testing.T, driver.Registry)
		d            string
		mw           func(http.Handler) http.Handler
		code         int
		header       http.Header
		expectHeader http.Header
		method       string
	}{
		{
			d:            "should ignore when disabled",
			prep:         func(t *testing.T, r driver.Registry) {},
			code:         http.StatusNotImplemented,
			header:       http.Header{},
			expectHeader: http.Header{},
		},
		{
			d: "should reject when basic auth but client does not exist and cors enabled",
			prep: func(t *testing.T, r driver.Registry) {
				r.Config().MustSet(context.Background(), "serve.public.cors.enabled", true)
				r.Config().MustSet(context.Background(), "serve.public.cors.allowed_origins", []string{"http://not-test-domain.com"})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://foobar.com"}, "Authorization": {fmt.Sprintf("Basic %s", x.BasicAuth("foo", "bar"))}},
			expectHeader: http.Header{"Vary": {"Origin"}},
		},
		{
			d: "should reject when basic auth client exists but origin not allowed",
			prep: func(t *testing.T, r driver.Registry) {
				r.Config().MustSet(context.Background(), "serve.public.cors.enabled", true)
				r.Config().MustSet(context.Background(), "serve.public.cors.allowed_origins", []string{"http://not-test-domain.com"})

				// Ignore unique violations
				_ = r.ClientManager().CreateClient(context.Background(), &client.Client{LegacyClientID: "foo-2", Secret: "bar", AllowedCORSOrigins: []string{"http://not-foobar.com"}})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://foobar.com"}, "Authorization": {fmt.Sprintf("Basic %s", x.BasicAuth("foo-2", "bar"))}},
			expectHeader: http.Header{"Vary": {"Origin"}},
		},
		{
			d: "should accept when basic auth client exists and origin allowed",
			prep: func(t *testing.T, r driver.Registry) {
				r.Config().MustSet(context.Background(), "serve.public.cors.enabled", true)

				// Ignore unique violations
				_ = r.ClientManager().CreateClient(context.Background(), &client.Client{LegacyClientID: "foo-3", Secret: "bar", AllowedCORSOrigins: []string{"http://foobar.com"}})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://foobar.com"}, "Authorization": {fmt.Sprintf("Basic %s", x.BasicAuth("foo-3", "bar"))}},
			expectHeader: http.Header{"Access-Control-Allow-Credentials": []string{"true"}, "Access-Control-Allow-Origin": []string{"http://foobar.com"}, "Access-Control-Expose-Headers": []string{"Cache-Control, Expires, Last-Modified, Pragma, Content-Length, Content-Language, Content-Type"}, "Vary": []string{"Origin"}},
		},
		{
			d: "should accept when basic auth client exists and origin allowed",
			prep: func(t *testing.T, r driver.Registry) {
				r.Config().MustSet(context.Background(), "serve.public.cors.enabled", true)
				r.Config().MustSet(context.Background(), "serve.public.cors.allowed_origins", []string{})

				// Ignore unique violations
				_ = r.ClientManager().CreateClient(context.Background(), &client.Client{LegacyClientID: "foo-3", Secret: "bar", AllowedCORSOrigins: []string{"http://foobar.com"}})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://foobar.com"}, "Authorization": {fmt.Sprintf("Basic %s", x.BasicAuth("foo-3", "bar"))}},
			expectHeader: http.Header{"Access-Control-Allow-Credentials": []string{"true"}, "Access-Control-Allow-Origin": []string{"http://foobar.com"}, "Access-Control-Expose-Headers": []string{"Cache-Control, Expires, Last-Modified, Pragma, Content-Length, Content-Language, Content-Type"}, "Vary": []string{"Origin"}},
		},
		{
			d: "should accept when basic auth client exists and origin (with partial wildcard) is allowed per client",
			prep: func(t *testing.T, r driver.Registry) {
				r.Config().MustSet(context.Background(), "serve.public.cors.enabled", true)
				r.Config().MustSet(context.Background(), "serve.public.cors.allowed_origins", []string{})

				// Ignore unique violations
				_ = r.ClientManager().CreateClient(context.Background(), &client.Client{LegacyClientID: "foo-4", Secret: "bar", AllowedCORSOrigins: []string{"http://*.foobar.com"}})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://foo.foobar.com"}, "Authorization": {fmt.Sprintf("Basic %s", x.BasicAuth("foo-4", "bar"))}},
			expectHeader: http.Header{"Access-Control-Allow-Credentials": []string{"true"}, "Access-Control-Allow-Origin": []string{"http://foo.foobar.com"}, "Access-Control-Expose-Headers": []string{"Cache-Control, Expires, Last-Modified, Pragma, Content-Length, Content-Language, Content-Type"}, "Vary": []string{"Origin"}},
		},
		{
			d: "should accept when basic auth client exists and origin (with full wildcard) is allowed globally",
			prep: func(t *testing.T, r driver.Registry) {
				r.Config().MustSet(context.Background(), "serve.public.cors.enabled", true)
				r.Config().MustSet(context.Background(), "serve.public.cors.allowed_origins", []string{"*"})

				// Ignore unique violations
				_ = r.ClientManager().CreateClient(context.Background(), &client.Client{LegacyClientID: "foo-5", Secret: "bar", AllowedCORSOrigins: []string{"http://barbar.com"}})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"*"}, "Authorization": {fmt.Sprintf("Basic %s", x.BasicAuth("foo-5", "bar"))}},
			expectHeader: http.Header{"Access-Control-Allow-Credentials": []string{"true"}, "Access-Control-Allow-Origin": []string{"*"}, "Access-Control-Expose-Headers": []string{"Cache-Control, Expires, Last-Modified, Pragma, Content-Length, Content-Language, Content-Type"}, "Vary": []string{"Origin"}},
		},
		{
			d: "should accept when basic auth client exists and origin (with partial wildcard) is allowed globally",
			prep: func(t *testing.T, r driver.Registry) {
				r.Config().MustSet(context.Background(), "serve.public.cors.enabled", true)
				r.Config().MustSet(context.Background(), "serve.public.cors.allowed_origins", []string{"http://*.foobar.com"})

				// Ignore unique violations
				_ = r.ClientManager().CreateClient(context.Background(), &client.Client{LegacyClientID: "foo-6", Secret: "bar", AllowedCORSOrigins: []string{"http://barbar.com"}})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://foo.foobar.com"}, "Authorization": {fmt.Sprintf("Basic %s", x.BasicAuth("foo-6", "bar"))}},
			expectHeader: http.Header{"Access-Control-Allow-Credentials": []string{"true"}, "Access-Control-Allow-Origin": []string{"http://foo.foobar.com"}, "Access-Control-Expose-Headers": []string{"Cache-Control, Expires, Last-Modified, Pragma, Content-Length, Content-Language, Content-Type"}, "Vary": []string{"Origin"}},
		},
		{
			d: "should accept when basic auth client exists and origin (with full wildcard) allowed per client",
			prep: func(t *testing.T, r driver.Registry) {
				r.Config().MustSet(context.Background(), "serve.public.cors.enabled", true)
				r.Config().MustSet(context.Background(), "serve.public.cors.allowed_origins", []string{"http://not-test-domain.com"})

				// Ignore unique violations
				_ = r.ClientManager().CreateClient(context.Background(), &client.Client{LegacyClientID: "foo-7", Secret: "bar", AllowedCORSOrigins: []string{"*"}})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://foobar.com"}, "Authorization": {fmt.Sprintf("Basic %s", x.BasicAuth("foo-7", "bar"))}},
			expectHeader: http.Header{"Access-Control-Allow-Credentials": []string{"true"}, "Access-Control-Allow-Origin": []string{"http://foobar.com"}, "Access-Control-Expose-Headers": []string{"Cache-Control, Expires, Last-Modified, Pragma, Content-Length, Content-Language, Content-Type"}, "Vary": []string{"Origin"}},
		},
		{
			d: "should succeed on pre-flight request when token introspection fails",
			prep: func(t *testing.T, r driver.Registry) {
				r.Config().MustSet(context.Background(), "serve.public.cors.enabled", true)
				r.Config().MustSet(context.Background(), "serve.public.cors.allowed_origins", []string{"http://not-test-domain.com"})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://foobar.com"}, "Authorization": {"Bearer 1234"}},
			expectHeader: http.Header{"Access-Control-Allow-Credentials": []string{"true"}, "Access-Control-Allow-Origin": []string{"http://foobar.com"}, "Access-Control-Expose-Headers": []string{"Cache-Control, Expires, Last-Modified, Pragma, Content-Length, Content-Language, Content-Type"}, "Vary": []string{"Origin"}},
			method:       "OPTIONS",
		},
		{
			d: "should fail when token introspection fails",
			prep: func(t *testing.T, r driver.Registry) {
				r.Config().MustSet(context.Background(), "serve.public.cors.enabled", true)
				r.Config().MustSet(context.Background(), "serve.public.cors.allowed_origins", []string{"http://not-test-domain.com"})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://foobar.com"}, "Authorization": {"Bearer 1234"}},
			expectHeader: http.Header{"Vary": {"Origin"}},
		},
		{
			d: "should work when token introspection returns a session",
			prep: func(t *testing.T, r driver.Registry) {
				r.Config().MustSet(context.Background(), "serve.public.cors.enabled", true)
				r.Config().MustSet(context.Background(), "serve.public.cors.allowed_origins", []string{"http://not-test-domain.com"})
				sess := oauth2.NewSession("foo-9")
				sess.SetExpiresAt(fosite.AccessToken, time.Now().Add(time.Hour))
				ar := fosite.NewAccessRequest(sess)
				cl := &client.Client{LegacyClientID: "foo-9", Secret: "bar", AllowedCORSOrigins: []string{"http://foobar.com"}}
				ar.Client = cl

				// Ignore unique violations
				_ = r.ClientManager().CreateClient(context.Background(), cl)
				_ = r.OAuth2Storage().CreateAccessTokenSession(context.Background(), signature, ar)
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://foobar.com"}, "Authorization": {"Bearer " + token}},
			expectHeader: http.Header{"Access-Control-Allow-Credentials": []string{"true"}, "Access-Control-Allow-Origin": []string{"http://foobar.com"}, "Access-Control-Expose-Headers": []string{"Cache-Control, Expires, Last-Modified, Pragma, Content-Length, Content-Language, Content-Type"}, "Vary": []string{"Origin"}},
		},
		{
			d: "should accept any allowed specified origin protocol",
			prep: func(t *testing.T, r driver.Registry) {
				r.Config().MustSet(context.Background(), "serve.public.cors.enabled", true)

				// Ignore unique violations
				_ = r.ClientManager().CreateClient(context.Background(), &client.Client{LegacyClientID: "foo-11", Secret: "bar", AllowedCORSOrigins: []string{"*"}})
				r.Config().MustSet(context.Background(), "serve.public.cors.enabled", true)
				r.Config().MustSet(context.Background(), "serve.public.cors.allowed_origins", []string{"http://*", "https://*"})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://foo.foobar.com"}, "Authorization": {fmt.Sprintf("Basic %s", x.BasicAuth("foo-11", "bar"))}},
			expectHeader: http.Header{"Access-Control-Allow-Credentials": []string{"true"}, "Access-Control-Allow-Origin": []string{"http://foo.foobar.com"}, "Access-Control-Expose-Headers": []string{"Cache-Control, Expires, Last-Modified, Pragma, Content-Length, Content-Language, Content-Type"}, "Vary": []string{"Origin"}},
		},
		{
			d: "should accept client origin when basic auth client exists and origin is set at the client as well as the server",
			prep: func(t *testing.T, r driver.Registry) {
				r.Config().MustSet(context.Background(), "serve.public.cors.enabled", true)
				r.Config().MustSet(context.Background(), "serve.public.cors.allowed_origins", []string{"http://**.example.com"})

				// Ignore unique violations
				_ = r.ClientManager().CreateClient(context.Background(), &client.Client{LegacyClientID: "foo-12", Secret: "bar", AllowedCORSOrigins: []string{"http://myapp.example.biz"}})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://myapp.example.biz"}, "Authorization": {fmt.Sprintf("Basic %s", x.BasicAuth("foo-12", "bar"))}},
			expectHeader: http.Header{"Access-Control-Allow-Credentials": []string{"true"}, "Access-Control-Allow-Origin": []string{"http://myapp.example.biz"}, "Access-Control-Expose-Headers": []string{"Cache-Control, Expires, Last-Modified, Pragma, Content-Length, Content-Language, Content-Type"}, "Vary": []string{"Origin"}},
		},
		{
			d: "should accept server origin when basic auth client exists and origin is set at the client as well as the server",
			prep: func(t *testing.T, r driver.Registry) {
				r.Config().MustSet(context.Background(), "serve.public.cors.enabled", true)
				r.Config().MustSet(context.Background(), "serve.public.cors.allowed_origins", []string{"http://**.example.com"})

				// Ignore unique violations
				_ = r.ClientManager().CreateClient(context.Background(), &client.Client{LegacyClientID: "foo-13", Secret: "bar", AllowedCORSOrigins: []string{"http://myapp.example.biz"}})
			},
			code:         http.StatusNotImplemented,
			header:       http.Header{"Origin": {"http://client-app.example.com"}, "Authorization": {fmt.Sprintf("Basic %s", x.BasicAuth("foo-13", "bar"))}},
			expectHeader: http.Header{"Access-Control-Allow-Credentials": []string{"true"}, "Access-Control-Allow-Origin": []string{"http://client-app.example.com"}, "Access-Control-Expose-Headers": []string{"Cache-Control, Expires, Last-Modified, Pragma, Content-Length, Content-Language, Content-Type"}, "Vary": []string{"Origin"}},
		},
	} {
		t.Run(fmt.Sprintf("case=%d/description=%s", k, tc.d), func(t *testing.T) {
			r.WithConfig(internal.NewConfigurationWithDefaults())

			if tc.prep != nil {
				tc.prep(t, r)
			}

			method := "GET"
			if tc.method != "" {
				method = tc.method
			}
			req, err := http.NewRequest(method, "http://foobar.com/", nil)
			require.NoError(t, err)
			for k := range tc.header {
				req.Header.Set(k, tc.header.Get(k))
			}

			res := httptest.NewRecorder()
			oauth2cors.Middleware(context.Background(), r)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotImplemented)
			})).ServeHTTP(res, req)
			require.NoError(t, err)
			assert.EqualValues(t, tc.code, res.Code)
			assert.EqualValues(t, tc.expectHeader, res.Header())
		})
	}
}
