// Copyright © 2022 Ory Corp
// SPDX-License-Identifier: Apache-2.0

package oauth2

import (
	"encoding/json"
	"time"

	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/mohae/deepcopy"

	"github.com/ory/fosite"
	"github.com/ory/fosite/handler/openid"
	"github.com/ory/fosite/token/jwt"

	"github.com/ory/x/stringslice"
)

// swagger:ignore
type Session struct {
	*openid.DefaultSession `json:"id_token"`
	Extra                  map[string]interface{} `json:"extra"`
	KID                    string                 `json:"kid"`
	ClientID               string                 `json:"client_id"`
	ConsentChallenge       string                 `json:"consent_challenge"`
	ExcludeNotBeforeClaim  bool                   `json:"exclude_not_before_claim"`
	AllowedTopLevelClaims  []string               `json:"allowed_top_level_claims"`
}

func NewSession(subject string) *Session {
	return NewSessionWithCustomClaims(subject, nil)
}

func NewSessionWithCustomClaims(subject string, allowedTopLevelClaims []string) *Session {
	return &Session{
		DefaultSession: &openid.DefaultSession{
			Claims:  new(jwt.IDTokenClaims),
			Headers: new(jwt.Headers),
			Subject: subject,
		},
		Extra:                 map[string]interface{}{},
		AllowedTopLevelClaims: allowedTopLevelClaims,
	}
}

func (s *Session) GetJWTClaims() jwt.JWTClaimsContainer {
	//a slice of claims that are reserved and should not be overridden
	var reservedClaims = []string{"iss", "sub", "aud", "exp", "nbf", "iat", "jti", "client_id", "scp", "ext"}

	//remove any reserved claims from the custom claims
	allowedClaimsFromConfigWithoutReserved := stringslice.Filter(s.AllowedTopLevelClaims, func(s string) bool {
		return stringslice.Has(reservedClaims, s)
	})

	//our new extra map which will be added to the jwt
	var topLevelExtraWithMirrorExt = map[string]interface{}{}

	//setting every allowed claim top level in jwt with respective value
	for _, allowedClaim := range allowedClaimsFromConfigWithoutReserved {
		if cl, ok := s.Extra[allowedClaim]; ok {
			topLevelExtraWithMirrorExt[allowedClaim] = cl
		}
	}

	//for every other claim that was already reserved and for mirroring, add original extra under "ext"
	topLevelExtraWithMirrorExt["ext"] = s.Extra

	claims := &jwt.JWTClaims{
		Subject: s.Subject,
		Issuer:  s.DefaultSession.Claims.Issuer,
		//set our custom extra map as claims.Extra
		Extra:     topLevelExtraWithMirrorExt,
		ExpiresAt: s.GetExpiresAt(fosite.AccessToken),
		IssuedAt:  time.Now(),

		// No need to set the audience because that's being done by fosite automatically.
		// Audience:  s.Audience,

		// The JTI MUST NOT BE FIXED or refreshing tokens will yield the SAME token
		// JTI:       s.JTI,

		// These are set by the DefaultJWTStrategy
		// Scope:     s.Scope,

		// Setting these here will cause the token to have the same iat/nbf values always
		// IssuedAt:  s.DefaultSession.Claims.IssuedAt,
		// NotBefore: s.DefaultSession.Claims.IssuedAt,
	}
	if !s.ExcludeNotBeforeClaim {
		claims.NotBefore = claims.IssuedAt
	}

	if claims.Extra == nil {
		claims.Extra = map[string]interface{}{}
	}

	claims.Extra["client_id"] = s.ClientID
	return claims
}

func (s *Session) GetJWTHeader() *jwt.Headers {
	return &jwt.Headers{
		Extra: map[string]interface{}{"kid": s.KID},
	}
}

func (s *Session) Clone() fosite.Session {
	if s == nil {
		return nil
	}

	return deepcopy.Copy(s).(fosite.Session)
}

var keyRewrites = map[string]string{
	"Extra":                          "extra",
	"KID":                            "kid",
	"ClientID":                       "client_id",
	"ConsentChallenge":               "consent_challenge",
	"ExcludeNotBeforeClaim":          "exclude_not_before_claim",
	"AllowedTopLevelClaims":          "allowed_top_level_claims",
	"idToken.Headers.Extra":          "id_token.headers.extra",
	"idToken.ExpiresAt":              "id_token.expires_at",
	"idToken.Username":               "id_token.username",
	"idToken.Subject":                "id_token.subject",
	"idToken.Claims.JTI":             "id_token.id_token_claims.jti",
	"idToken.Claims.Issuer":          "id_token.id_token_claims.iss",
	"idToken.Claims.Subject":         "id_token.id_token_claims.sub",
	"idToken.Claims.Audience":        "id_token.id_token_claims.aud",
	"idToken.Claims.Nonce":           "id_token.id_token_claims.nonce",
	"idToken.Claims.ExpiresAt":       "id_token.id_token_claims.exp",
	"idToken.Claims.IssuedAt":        "id_token.id_token_claims.iat",
	"idToken.Claims.RequestedAt":     "id_token.id_token_claims.rat",
	"idToken.Claims.AuthTime":        "id_token.id_token_claims.auth_time",
	"idToken.Claims.AccessTokenHash": "id_token.id_token_claims.at_hash",
	"idToken.Claims.AuthenticationContextClassReference": "id_token.id_token_claims.acr",
	"idToken.Claims.AuthenticationMethodsReferences":     "id_token.id_token_claims.amr",
	"idToken.Claims.CodeHash":                            "id_token.id_token_claims.c_hash",
	"idToken.Claims.Extra":                               "id_token.id_token_claims.ext",
}

func (s *Session) UnmarshalJSON(original []byte) (err error) {
	transformed := original
	originalParsed := gjson.ParseBytes(original)

	for oldKey, newKey := range keyRewrites {
		if !originalParsed.Get(oldKey).Exists() {
			continue
		}
		transformed, err = sjson.SetRawBytes(transformed, newKey, []byte(originalParsed.Get(oldKey).Raw))
		if err != nil {
			return errors.WithStack(err)
		}
	}

	for orig := range keyRewrites {
		transformed, err = sjson.DeleteBytes(transformed, orig)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	if originalParsed.Get("idToken").Exists() {
		transformed, err = sjson.DeleteBytes(transformed, "idToken")
		if err != nil {
			return errors.WithStack(err)
		}
	}

	type t Session
	if err := json.Unmarshal(transformed, (*t)(s)); err != nil {
		return errors.WithStack(err)
	}

	return nil
}
