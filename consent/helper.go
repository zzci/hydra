// Copyright © 2022 Ory Corp
// SPDX-License-Identifier: Apache-2.0

package consent

import (
	"net/http"
	"strings"

	"time"

	"github.com/ory/hydra/x"

	"github.com/ory/x/errorsx"

	"github.com/gorilla/sessions"

	"github.com/ory/fosite"
	"github.com/ory/x/mapx"

	"github.com/ory/hydra/client"
)

func sanitizeClientFromRequest(ar fosite.AuthorizeRequester) *client.Client {
	return sanitizeClient(ar.GetClient().(*client.Client))
}

func sanitizeClient(c *client.Client) *client.Client {
	cc := new(client.Client)
	// Remove the hashed secret here
	*cc = *c
	cc.Secret = ""
	return cc
}

func matchScopes(scopeStrategy fosite.ScopeStrategy, previousConsent []AcceptOAuth2ConsentRequest, requestedScope []string) *AcceptOAuth2ConsentRequest {
	for _, cs := range previousConsent {
		var found = true
		for _, scope := range requestedScope {
			if !scopeStrategy(cs.GrantedScope, scope) {
				found = false
				break
			}
		}

		if found {
			return &cs
		}
	}

	return nil
}

func createCsrfSession(w http.ResponseWriter, r *http.Request, conf x.CookieConfigProvider, store sessions.Store, name string, csrfValue string, maxAge time.Duration) error {
	// Errors can be ignored here, because we always get a session back. Error typically means that the
	// session doesn't exist yet.
	session, _ := store.Get(r, name)

	sameSite := conf.CookieSameSiteMode(r.Context())
	if isLegacyCsrfSessionName(name) {
		sameSite = 0
	}

	session.Values["csrf"] = csrfValue
	session.Options.HttpOnly = true
	session.Options.Secure = conf.CookieSecure(r.Context())
	session.Options.SameSite = sameSite
	session.Options.Domain = conf.CookieDomain(r.Context())
	session.Options.MaxAge = int(maxAge.Seconds())
	if err := session.Save(r, w); err != nil {
		return errorsx.WithStack(err)
	}

	if sameSite == http.SameSiteNoneMode && conf.CookieSameSiteLegacyWorkaround(r.Context()) {
		return createCsrfSession(w, r, conf, store, legacyCsrfSessionName(name), csrfValue, maxAge)
	}

	return nil
}

func validateCsrfSession(r *http.Request, conf x.CookieConfigProvider, store sessions.Store, name, expectedCSRF string) error {
	if cookie, err := getCsrfSession(r, store, conf, name); err != nil {
		return errorsx.WithStack(fosite.ErrRequestForbidden.WithHint("CSRF session cookie could not be decoded."))
	} else if csrf, err := mapx.GetString(cookie.Values, "csrf"); err != nil {
		return errorsx.WithStack(fosite.ErrRequestForbidden.WithHint("No CSRF value available in the session cookie."))
	} else if csrf != expectedCSRF {
		return errorsx.WithStack(fosite.ErrRequestForbidden.WithHint("The CSRF value from the token does not match the CSRF value from the data store."))
	}

	return nil
}

func getCsrfSession(r *http.Request, store sessions.Store, conf x.CookieConfigProvider, name string) (*sessions.Session, error) {
	cookie, err := store.Get(r, name)
	if !isLegacyCsrfSessionName(name) && conf.CookieSameSiteMode(r.Context()) == http.SameSiteNoneMode && conf.CookieSameSiteLegacyWorkaround(r.Context()) && (err != nil || len(cookie.Values) == 0) {
		return store.Get(r, legacyCsrfSessionName(name))
	}
	return cookie, err
}

func legacyCsrfSessionName(name string) string {
	return name + "_legacy"
}

func isLegacyCsrfSessionName(name string) bool {
	return strings.HasSuffix(name, "_legacy")
}
