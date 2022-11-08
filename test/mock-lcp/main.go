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
 * @Copyright 	2017-2018 Aeneas Rekkas <aeneas+oss@aeneas.io>
 * @license 	Apache-2.0
 */

package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	hydra "github.com/ory/hydra-client-go"

	"github.com/ory/x/pointerx"
	"github.com/ory/x/urlx"
)

var hydraURL = urlx.ParseOrPanic(os.Getenv("HYDRA_ADMIN_URL"))
var client = hydra.NewAPIClient(hydra.NewConfiguration())

func init() {
	client.GetConfig().Servers = hydra.ServerConfigurations{{URL: hydraURL.String()}}
}

func login(rw http.ResponseWriter, r *http.Request) {
	challenge := r.URL.Query().Get("login_challenge")
	lr, resp, err := client.OAuth2Api.GetOAuth2LoginRequest(r.Context()).LoginChallenge(challenge).Execute()
	defer resp.Body.Close()
	if err != nil {
		log.Fatalf("Unable to fetch clogin request: %s", err)
	}

	var redirectTo string
	if strings.Contains(lr.RequestUrl, "mockLogin=accept") {
		remember := false
		if strings.Contains(lr.RequestUrl, "rememberLogin=yes") {
			remember = true
		}

		vr, resp, err := client.OAuth2Api.AcceptOAuth2LoginRequest(r.Context()).
			LoginChallenge(challenge).
			AcceptOAuth2LoginRequest(hydra.AcceptOAuth2LoginRequest{
				Subject:  "the-subject",
				Remember: pointerx.Bool(remember),
			}).Execute()
		defer resp.Body.Close()
		if err != nil {
			log.Fatalf("Unable to execute request: %s", err)
		}
		redirectTo = vr.RedirectTo
	} else {
		vr, resp, err := client.OAuth2Api.RejectOAuth2LoginRequest(r.Context()).
			LoginChallenge(challenge).
			RejectOAuth2Request(hydra.RejectOAuth2Request{
				Error: pointerx.String("invalid_request"),
			}).Execute()
		defer resp.Body.Close()
		if err != nil {
			log.Fatalf("Unable to execute request: %s", err)
		}
		redirectTo = vr.RedirectTo
	}
	if err != nil {
		log.Fatalf("Unable to accept/reject login request: %s", err)
	}
	http.Redirect(rw, r, redirectTo, http.StatusFound)
}

func consent(rw http.ResponseWriter, r *http.Request) {
	challenge := r.URL.Query().Get("consent_challenge")

	o, resp, err := client.OAuth2Api.GetOAuth2ConsentRequest(r.Context()).ConsentChallenge(challenge).Execute()
	defer resp.Body.Close()
	if err != nil {
		log.Fatalf("Unable to fetch consent request: %s", err)
	}

	var redirectTo string
	if strings.Contains(*o.RequestUrl, "mockConsent=accept") {
		remember := false
		if strings.Contains(*o.RequestUrl, "rememberConsent=yes") {
			remember = true
		}
		value := "bar"
		if *o.Skip {
			value = "rab"
		}

		v, resp, err := client.OAuth2Api.AcceptOAuth2ConsentRequest(r.Context()).
			ConsentChallenge(challenge).
			AcceptOAuth2ConsentRequest(hydra.AcceptOAuth2ConsentRequest{
				GrantScope: o.RequestedScope,
				Remember:   pointerx.Bool(remember),
				Session: &hydra.AcceptOAuth2ConsentRequestSession{
					AccessToken: map[string]interface{}{"foo": value},
					IdToken:     map[string]interface{}{"baz": value},
				},
			}).Execute()
		defer resp.Body.Close()
		if err != nil {
			log.Fatalf("Unable to execute request: %s", err)
		}
		redirectTo = v.RedirectTo
	} else {
		v, resp, err := client.OAuth2Api.RejectOAuth2ConsentRequest(r.Context()).
			ConsentChallenge(challenge).
			RejectOAuth2Request(hydra.RejectOAuth2Request{Error: pointerx.String("invalid_request")}).Execute()
		defer resp.Body.Close()
		if err != nil {
			log.Fatalf("Unable to execute request: %s", err)
		}
		redirectTo = v.RedirectTo
	}
	if err != nil {
		log.Fatalf("Unable to accept/reject consent request: %s", err)
	}

	http.Redirect(rw, r, redirectTo, http.StatusFound)
}

func errh(rw http.ResponseWriter, r *http.Request) {
	http.Error(rw, r.URL.Query().Get("error")+" "+r.URL.Query().Get("error_debug"), http.StatusInternalServerError)
}

func main() {
	http.HandleFunc("/login", login)
	http.HandleFunc("/consent", consent)
	http.HandleFunc("/error", errh)
	port := "3000"
	if os.Getenv("PORT") != "" {
		port = os.Getenv("PORT")
	}
	log.Fatal(http.ListenAndServe(":"+port, nil)) // #nosec G114
}
