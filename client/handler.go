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

package client

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ory/x/pagination/tokenpagination"

	"github.com/ory/x/httprouterx"

	"github.com/ory/x/openapix"

	"github.com/ory/x/uuidx"

	"github.com/ory/x/jsonx"
	"github.com/ory/x/urlx"

	"github.com/ory/fosite"

	"github.com/ory/x/errorsx"

	"github.com/ory/herodot"
	"github.com/ory/hydra/x"

	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

type Handler struct {
	r InternalRegistry
}

const (
	ClientsHandlerPath    = "/clients"
	DynClientsHandlerPath = "/oauth2/register"
)

func NewHandler(r InternalRegistry) *Handler {
	return &Handler{
		r: r,
	}
}

func (h *Handler) SetRoutes(admin *httprouterx.RouterAdmin, public *httprouterx.RouterPublic) {
	admin.GET(ClientsHandlerPath, h.listOAuth2Clients)
	admin.POST(ClientsHandlerPath, h.createOAuth2Client)
	admin.GET(ClientsHandlerPath+"/:id", h.Get)
	admin.PUT(ClientsHandlerPath+"/:id", h.setOAuth2Client)
	admin.PATCH(ClientsHandlerPath+"/:id", h.patchOAuth2Client)
	admin.DELETE(ClientsHandlerPath+"/:id", h.deleteOAuth2Client)
	admin.PUT(ClientsHandlerPath+"/:id/lifespans", h.setOAuth2ClientLifespans)

	public.POST(DynClientsHandlerPath, h.createOidcDynamicClient)
	public.GET(DynClientsHandlerPath+"/:id", h.getOidcDynamicClient)
	public.PUT(DynClientsHandlerPath+"/:id", h.setOidcDynamicClient)
	public.DELETE(DynClientsHandlerPath+"/:id", h.deleteOidcDynamicClient)
}

// OAuth 2.0 Client Creation Parameters
//
// swagger:parameters createOAuth2Client
type createOAuth2Client struct {
	// OAuth 2.0 Client Request Body
	//
	// in: body
	// required: true
	Body Client
}

// swagger:route POST /admin/clients oAuth2 createOAuth2Client
//
// # Create OAuth 2.0 Client
//
// Create a new OAuth 2.0 client. If you pass `client_secret` the secret is used, otherwise a random secret
// is generated. The secret is echoed in the response. It is not possible to retrieve it later on.
//
//	Consumes:
//	- application/json
//
//	Produces:
//	- application/json
//
//	Schemes: http, https
//
//	Responses:
//	  201: oAuth2Client
//	  400: errorOAuth2BadRequest
//	  default: errorOAuth2Default
func (h *Handler) createOAuth2Client(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	c, err := h.CreateClient(r, h.r.ClientValidator().Validate, false)
	if err != nil {
		h.r.Writer().WriteError(w, r, errorsx.WithStack(err))
		return
	}

	h.r.Writer().WriteCreated(w, r, "/admin"+ClientsHandlerPath+"/"+c.GetID(), &c)
}

// OpenID Connect Dynamic Client Registration Parameters
//
// swagger:parameters createOidcDynamicClient
type createOidcDynamicClient struct {
	// Dynamic Client Registration Request Body
	//
	// in: body
	// required: true
	Body Client
}

// swagger:route POST /oauth2/register oidc createOidcDynamicClient
//
// # Register OAuth2 Client using OpenID Dynamic Client Registration
//
// This endpoint behaves like the administrative counterpart (`createOAuth2Client`) but is capable of facing the
// public internet directly and can be used in self-service. It implements the OpenID Connect
// Dynamic Client Registration Protocol. This feature needs to be enabled in the configuration. This endpoint
// is disabled by default. It can be enabled by an administrator.
//
// Please note that using this endpoint you are not able to choose the `client_secret` nor the `client_id` as those
// values will be server generated when specifying `token_endpoint_auth_method` as `client_secret_basic` or
// `client_secret_post`.
//
// The `client_secret` will be returned in the response and you will not be able to retrieve it later on.
// Write the secret down and keep it somewhere safe.
//
//	Consumes:
//	- application/json
//
//	Produces:
//	- application/json
//
//	Schemes: http, https
//
//	Responses:
//	  201: oAuth2Client
//	  400: errorOAuth2BadRequest
//	  default: errorOAuth2Default
func (h *Handler) createOidcDynamicClient(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	if err := h.requireDynamicAuth(r); err != nil {
		h.r.Writer().WriteError(w, r, err)
		return
	}
	c, err := h.CreateClient(r, h.r.ClientValidator().ValidateDynamicRegistration, true)
	if err != nil {
		h.r.Writer().WriteError(w, r, errorsx.WithStack(err))
		return
	}

	h.r.Writer().WriteCreated(w, r, "/admin"+ClientsHandlerPath+"/"+c.GetID(), &c)
}

func (h *Handler) CreateClient(r *http.Request, validator func(context.Context, *Client) error, isDynamic bool) (*Client, error) {
	var c Client
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		return nil, err
	}

	if isDynamic {
		if c.Secret != "" {
			return nil, errorsx.WithStack(herodot.ErrBadRequest.WithReasonf("It is not allowed to choose your own OAuth2 Client secret."))
		}
	}

	if len(c.LegacyClientID) > 0 {
		return nil, errorsx.WithStack(herodot.ErrBadRequest.WithReason("It is no longer possible to set an OAuth2 Client ID as a user. The system will generate a unique ID for you."))
	}

	c.ID = uuidx.NewV4()
	c.LegacyClientID = c.ID.String()

	if len(c.Secret) == 0 {
		secretb, err := x.GenerateSecret(26)
		if err != nil {
			return nil, err
		}
		c.Secret = string(secretb)
	}

	if err := validator(r.Context(), &c); err != nil {
		return nil, err
	}

	secret := c.Secret
	c.CreatedAt = time.Now().UTC().Round(time.Second)
	c.UpdatedAt = c.CreatedAt

	token, signature, err := h.r.OAuth2HMACStrategy().GenerateAccessToken(r.Context(), nil)
	if err != nil {
		return nil, err
	}

	c.RegistrationAccessToken = token
	c.RegistrationAccessTokenSignature = signature
	c.RegistrationClientURI = urlx.AppendPaths(h.r.Config().PublicURL(r.Context()), DynClientsHandlerPath+"/"+c.GetID()).String()

	if err := h.r.ClientManager().CreateClient(r.Context(), &c); err != nil {
		return nil, err
	}
	c.Secret = ""
	if !c.IsPublic() {
		c.Secret = secret
	}
	return &c, nil
}

// Set OAuth 2.0 Client Parameters
//
// swagger:parameters setOAuth2Client
type setOAuth2Client struct {
	// OAuth 2.0 Client ID
	//
	// in: path
	// required: true
	ID string `json:"id"`

	// OAuth 2.0 Client Request Body
	//
	// in: body
	// required: true
	Body Client
}

// swagger:route PUT /admin/clients/{id} oAuth2 setOAuth2Client
//
// # Set OAuth 2.0 Client
//
// Replaces an existing OAuth 2.0 Client with the payload you send. If you pass `client_secret` the secret is used,
// otherwise the existing secret is used.
//
// If set, the secret is echoed in the response. It is not possible to retrieve it later on.
//
// OAuth 2.0 Clients are used to perform OAuth 2.0 and OpenID Connect flows. Usually, OAuth 2.0 clients are
// generated for applications which want to consume your OAuth 2.0 or OpenID Connect capabilities.
//
//	Consumes:
//	- application/json
//
//	Produces:
//	- application/json
//
//	Schemes: http, https
//
//	Responses:
//	  200: oAuth2Client
//	  400: errorOAuth2BadRequest
//	  404: errorOAuth2NotFound
//	  default: errorOAuth2Default
func (h *Handler) setOAuth2Client(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	var c Client
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		h.r.Writer().WriteError(w, r, errorsx.WithStack(herodot.ErrBadRequest.WithReasonf("Unable to decode the request body: %s", err)))
		return
	}

	c.LegacyClientID = ps.ByName("id")
	if err := h.updateClient(r.Context(), &c, h.r.ClientValidator().Validate); err != nil {
		h.r.Writer().WriteError(w, r, err)
		return
	}

	h.r.Writer().Write(w, r, &c)
}

func (h *Handler) updateClient(ctx context.Context, c *Client, validator func(context.Context, *Client) error) error {
	var secret string
	if len(c.Secret) > 0 {
		secret = c.Secret
	}

	if err := validator(ctx, c); err != nil {
		return err
	}

	c.UpdatedAt = time.Now().UTC().Round(time.Second)
	if err := h.r.ClientManager().UpdateClient(ctx, c); err != nil {
		return err
	}
	c.Secret = secret
	return nil
}

// Set Dynamic Client Parameters
//
// swagger:parameters setOidcDynamicClient
type setOidcDynamicClient struct {
	// OAuth 2.0 Client ID
	//
	// in: path
	// required: true
	ID string `json:"id"`

	// OAuth 2.0 Client Request Body
	//
	// in: body
	// required: true
	Body Client
}

// swagger:route PUT /oauth2/register/{id} oidc setOidcDynamicClient
//
// # Set OAuth2 Client using OpenID Dynamic Client Registration
//
// This endpoint behaves like the administrative counterpart (`setOAuth2Client`) but is capable of facing the
// public internet directly to be used by third parties. It implements the OpenID Connect
// Dynamic Client Registration Protocol.
//
// This feature is disabled per default. It can be enabled by a system administrator.
//
// If you pass `client_secret` the secret is used, otherwise the existing secret is used. If set, the secret is echoed in the response.
// It is not possible to retrieve it later on.
//
// To use this endpoint, you will need to present the client's authentication credentials. If the OAuth2 Client
// uses the Token Endpoint Authentication Method `client_secret_post`, you need to present the client secret in the URL query.
// If it uses `client_secret_basic`, present the Client ID and the Client Secret in the Authorization header.
//
// OAuth 2.0 clients are used to perform OAuth 2.0 and OpenID Connect flows. Usually, OAuth 2.0 clients are
// generated for applications which want to consume your OAuth 2.0 or OpenID Connect capabilities.
//
//	Consumes:
//	- application/json
//
//	Produces:
//	- application/json
//
//	Security:
//	  bearer:
//
//	Schemes: http, https
//
//	Responses:
//	  200: oAuth2Client
//	  404: errorOAuth2NotFound
//	  default: errorOAuth2Default
func (h *Handler) setOidcDynamicClient(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	if err := h.requireDynamicAuth(r); err != nil {
		h.r.Writer().WriteError(w, r, err)
		return
	}

	client, err := h.ValidDynamicAuth(r, ps)
	if err != nil {
		h.r.Writer().WriteError(w, r, err)
		return
	}

	var c Client
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		h.r.Writer().WriteError(w, r, errorsx.WithStack(herodot.ErrBadRequest.WithReasonf("Unable to decode the request body. Is it valid JSON?").WithDebug(err.Error())))
		return
	}

	if c.Secret != "" {
		h.r.Writer().WriteError(w, r, errorsx.WithStack(herodot.ErrForbidden.WithReasonf("It is not allowed to choose your own OAuth2 Client secret.")))
		return
	}

	// Regenerate the registration access token
	token, signature, err := h.r.OAuth2HMACStrategy().GenerateAccessToken(r.Context(), nil)
	if err != nil {
		h.r.Writer().WriteError(w, r, err)
		return
	}
	c.RegistrationAccessToken = token
	c.RegistrationAccessTokenSignature = signature

	c.LegacyClientID = client.GetID()
	if err := h.updateClient(r.Context(), &c, h.r.ClientValidator().ValidateDynamicRegistration); err != nil {
		h.r.Writer().WriteError(w, r, err)
		return
	}

	h.r.Writer().Write(w, r, &c)
}

// Patch OAuth 2.0 Client Parameters
//
// swagger:parameters patchOAuth2Client
type patchOAuth2Client struct {
	// The id of the OAuth 2.0 Client.
	//
	// in: path
	// required: true
	ID string `json:"id"`

	// OAuth 2.0 Client JSON Patch Body
	//
	// in: body
	// required: true
	Body openapix.JSONPatchDocument
}

// swagger:route PATCH /admin/clients/{id} oAuth2 patchOAuth2Client
//
// # Patch OAuth 2.0 Client
//
// Patch an existing OAuth 2.0 Client using JSON Patch. If you pass `client_secret`
// the secret will be updated and returned via the API. This is the
// only time you will be able to retrieve the client secret, so write it down and keep it safe.
//
// OAuth 2.0 clients are used to perform OAuth 2.0 and OpenID Connect flows. Usually, OAuth 2.0 clients are
// generated for applications which want to consume your OAuth 2.0 or OpenID Connect capabilities.
//
//	Consumes:
//	- application/json
//
//	Produces:
//	- application/json
//
//	Schemes: http, https
//
//	Responses:
//	  200: oAuth2Client
//	  404: errorOAuth2NotFound
//	  default: errorOAuth2Default
func (h *Handler) patchOAuth2Client(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	patchJSON, err := io.ReadAll(r.Body)
	if err != nil {
		h.r.Writer().WriteError(w, r, err)
		return
	}

	id := ps.ByName("id")
	c, err := h.r.ClientManager().GetConcreteClient(r.Context(), id)
	if err != nil {
		h.r.Writer().WriteError(w, r, err)
		return
	}

	oldSecret := c.Secret

	if err := jsonx.ApplyJSONPatch(patchJSON, c, "/id"); err != nil {
		h.r.Writer().WriteError(w, r, err)
		return
	}

	// fix for #2869
	// GetConcreteClient returns a client with the hashed secret, however updateClient expects
	// an empty secret if the secret hasn't changed. As such we need to check if the patch has
	// updated the secret or not
	if oldSecret == c.Secret {
		c.Secret = ""
	}

	if err := h.updateClient(r.Context(), c, h.r.ClientValidator().Validate); err != nil {
		h.r.Writer().WriteError(w, r, err)
		return
	}

	h.r.Writer().Write(w, r, c)
}

// Paginated OAuth2 Client List Response
//
// swagger:response listOAuth2Clients
type listOAuth2ClientsResponse struct {
	tokenpagination.ResponseHeaders

	// List of OAuth 2.0 Clients
	//
	// in:body
	Body []Client
}

// Paginated OAuth2 Client List Parameters
//
// swagger:parameters listOAuth2Clients
type listOAuth2ClientsParameters struct {
	tokenpagination.RequestParameters

	// The name of the clients to filter by.
	//
	// in: query
	Name string `json:"client_name"`

	// The owner of the clients to filter by.
	//
	// in: query
	Owner string `json:"owner"`
}

// swagger:route GET /admin/clients oAuth2 listOAuth2Clients
//
// # List OAuth 2.0 Clients
//
// This endpoint lists all clients in the database, and never returns client secrets.
// As a default it lists the first 100 clients.
//
//	Consumes:
//	- application/json
//
//	Produces:
//	- application/json
//
//	Schemes: http, https
//
//	Responses:
//	  200: listOAuth2Clients
//	  default: errorOAuth2Default
func (h *Handler) listOAuth2Clients(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	page, itemsPerPage := x.ParsePagination(r)
	filters := Filter{
		Limit:  itemsPerPage,
		Offset: page * itemsPerPage,
		Name:   r.URL.Query().Get("client_name"),
		Owner:  r.URL.Query().Get("owner"),
	}

	c, err := h.r.ClientManager().GetClients(r.Context(), filters)
	if err != nil {
		h.r.Writer().WriteError(w, r, err)
		return
	}

	if c == nil {
		c = []Client{}
	}

	for k := range c {
		c[k].Secret = ""
	}

	total, err := h.r.ClientManager().CountClients(r.Context())
	if err != nil {
		h.r.Writer().WriteError(w, r, err)
		return
	}

	x.PaginationHeader(w, r.URL, int64(total), page, itemsPerPage)
	h.r.Writer().Write(w, r, c)
}

// Get OAuth2 Client Parameters
//
// swagger:parameters getOAuth2Client
type adminGetOAuth2Client struct {
	// The id of the OAuth 2.0 Client.
	//
	// in: path
	// required: true
	ID string `json:"id"`
}

// swagger:route GET /admin/clients/{id} oAuth2 getOAuth2Client
//
// # Get an OAuth 2.0 Client
//
// Get an OAuth 2.0 client by its ID. This endpoint never returns the client secret.
//
// OAuth 2.0 clients are used to perform OAuth 2.0 and OpenID Connect flows. Usually, OAuth 2.0 clients are
// generated for applications which want to consume your OAuth 2.0 or OpenID Connect capabilities.
//
//	Consumes:
//	- application/json
//
//	Produces:
//	- application/json
//
//	Schemes: http, https
//
//	Responses:
//	  200: oAuth2Client
//	  default: errorOAuth2Default
func (h *Handler) Get(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	var id = ps.ByName("id")
	c, err := h.r.ClientManager().GetConcreteClient(r.Context(), id)
	if err != nil {
		h.r.Writer().WriteError(w, r, err)
		return
	}

	c.Secret = ""
	h.r.Writer().Write(w, r, c)
}

// Get OpenID Connect Dynamic Client Parameters
//
// swagger:parameters getOidcDynamicClient
type getOidcDynamicClient struct {
	// The id of the OAuth 2.0 Client.
	//
	// in: path
	// required: true
	ID string `json:"id"`
}

// swagger:route GET /oauth2/register/{id} oidc getOidcDynamicClient
//
// # Get OAuth2 Client using OpenID Dynamic Client Registration
//
// This endpoint behaves like the administrative counterpart (`getOAuth2Client`) but is capable of facing the
// public internet directly and can be used in self-service. It implements the OpenID Connect
// Dynamic Client Registration Protocol.
//
// To use this endpoint, you will need to present the client's authentication credentials. If the OAuth2 Client
// uses the Token Endpoint Authentication Method `client_secret_post`, you need to present the client secret in the URL query.
// If it uses `client_secret_basic`, present the Client ID and the Client Secret in the Authorization header.
//
//	Consumes:
//	- application/json
//
//	Produces:
//	- application/json
//
//	Schemes: http, https
//
//	Security:
//	  bearer:
//
//	Responses:
//	  200: oAuth2Client
//	  default: errorOAuth2Default
func (h *Handler) getOidcDynamicClient(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	if err := h.requireDynamicAuth(r); err != nil {
		h.r.Writer().WriteError(w, r, err)
		return
	}

	client, err := h.ValidDynamicAuth(r, ps)
	if err != nil {
		h.r.Writer().WriteError(w, r, err)
		return
	}

	c, err := h.r.ClientManager().GetConcreteClient(r.Context(), client.GetID())
	if err != nil {
		err = herodot.ErrUnauthorized.WithReason("The requested OAuth 2.0 client does not exist or you did not provide the necessary credentials")
		h.r.Writer().WriteError(w, r, err)
		return
	}

	c.Secret = ""
	c.Metadata = nil
	h.r.Writer().Write(w, r, c)
}

// Delete OAuth2 Client Parameters
//
// swagger:parameters deleteOAuth2Client
type deleteOAuth2Client struct {
	// The id of the OAuth 2.0 Client.
	//
	// in: path
	// required: true
	ID string `json:"id"`
}

// swagger:route DELETE /admin/clients/{id} oAuth2 deleteOAuth2Client
//
// # Delete OAuth 2.0 Client
//
// Delete an existing OAuth 2.0 Client by its ID.
//
// OAuth 2.0 clients are used to perform OAuth 2.0 and OpenID Connect flows. Usually, OAuth 2.0 clients are
// generated for applications which want to consume your OAuth 2.0 or OpenID Connect capabilities.
//
// Make sure that this endpoint is well protected and only callable by first-party components.
//
//	Consumes:
//	- application/json
//
//	Produces:
//	- application/json
//
//	Schemes: http, https
//
//	Responses:
//	  204: emptyResponse
//	  default: genericError
func (h *Handler) deleteOAuth2Client(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	var id = ps.ByName("id")
	if err := h.r.ClientManager().DeleteClient(r.Context(), id); err != nil {
		h.r.Writer().WriteError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Set OAuth 2.0 Client Token Lifespans
//
// swagger:parameters setOAuth2ClientLifespans
type setOAuth2ClientLifespans struct {
	// OAuth 2.0 Client ID
	//
	// in: path
	// required: true
	ID string `json:"id"`

	// in: body
	Body Lifespans
}

// swagger:route PUT /admin/clients/{id}/lifespans oAuth2 setOAuth2ClientLifespans
//
// # Set OAuth2 Client Token Lifespans
//
// Set lifespans of different token types issued for this OAuth 2.0 client. Does not modify other fields.
//
//	Consumes:
//	- application/json
//
//	Schemes: http, https
//
//	Responses:
//	  200: oAuth2Client
//	  default: genericError
func (h *Handler) setOAuth2ClientLifespans(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	var id = ps.ByName("id")
	c, err := h.r.ClientManager().GetConcreteClient(r.Context(), id)
	if err != nil {
		h.r.Writer().WriteError(w, r, err)
		return
	}

	var ls Lifespans
	if err := json.NewDecoder(r.Body).Decode(&ls); err != nil {
		h.r.Writer().WriteError(w, r, errorsx.WithStack(herodot.ErrBadRequest.WithReasonf("Unable to decode the request body: %s", err)))
		return
	}

	c.Lifespans = ls
	c.Secret = ""

	if err := h.updateClient(r.Context(), c, h.r.ClientValidator().Validate); err != nil {
		h.r.Writer().WriteError(w, r, err)
		return
	}

	h.r.Writer().Write(w, r, c)
}

// swagger:parameters deleteOidcDynamicClient
type dynamicClientRegistrationDeleteOAuth2Client struct {
	// The id of the OAuth 2.0 Client.
	//
	// in: path
	// required: true
	ID string `json:"id"`
}

// swagger:route DELETE /oauth2/register/{id} oidc deleteOidcDynamicClient
//
// # Delete OAuth 2.0 Client using the OpenID Dynamic Client Registration Management Protocol
//
// This endpoint behaves like the administrative counterpart (`deleteOAuth2Client`) but is capable of facing the
// public internet directly and can be used in self-service. It implements the OpenID Connect
// Dynamic Client Registration Protocol. This feature needs to be enabled in the configuration. This endpoint
// is disabled by default. It can be enabled by an administrator.
//
// To use this endpoint, you will need to present the client's authentication credentials. If the OAuth2 Client
// uses the Token Endpoint Authentication Method `client_secret_post`, you need to present the client secret in the URL query.
// If it uses `client_secret_basic`, present the Client ID and the Client Secret in the Authorization header.
//
// OAuth 2.0 clients are used to perform OAuth 2.0 and OpenID Connect flows. Usually, OAuth 2.0 clients are
// generated for applications which want to consume your OAuth 2.0 or OpenID Connect capabilities.
//
//	Produces:
//	- application/json
//
//	Schemes: http, https
//
//	Security:
//	  bearer:
//
//	Responses:
//	  204: emptyResponse
//	  default: genericError
func (h *Handler) deleteOidcDynamicClient(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	if err := h.requireDynamicAuth(r); err != nil {
		h.r.Writer().WriteError(w, r, err)
		return
	}
	client, err := h.ValidDynamicAuth(r, ps)
	if err != nil {
		h.r.Writer().WriteError(w, r, err)
		return
	}

	if err := h.r.ClientManager().DeleteClient(r.Context(), client.GetID()); err != nil {
		h.r.Writer().WriteError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ValidDynamicAuth(r *http.Request, ps httprouter.Params) (fosite.Client, error) {
	c, err := h.r.ClientManager().GetConcreteClient(r.Context(), ps.ByName("id"))
	if err != nil {
		return nil, herodot.ErrUnauthorized.
			WithTrace(err).
			WithReason("The requested OAuth 2.0 client does not exist or you provided incorrect credentials.").WithDebug(err.Error())
	}

	if len(c.RegistrationAccessTokenSignature) == 0 {
		return nil, errors.WithStack(herodot.ErrUnauthorized.
			WithReason("The requested OAuth 2.0 client does not exist or you provided incorrect credentials.").WithDebug("The OAuth2 Client does not have a registration access token."))
	}

	token := strings.TrimPrefix(fosite.AccessTokenFromRequest(r), "ory_at_")
	if err := h.r.OAuth2HMACStrategy().Enigma.Validate(r.Context(), token); err != nil {
		return nil, herodot.ErrUnauthorized.
			WithTrace(err).
			WithReason("The requested OAuth 2.0 client does not exist or you provided incorrect credentials.").WithDebug(err.Error())
	}

	signature := h.r.OAuth2HMACStrategy().Enigma.Signature(token)
	if subtle.ConstantTimeCompare([]byte(c.RegistrationAccessTokenSignature), []byte(signature)) == 0 {
		return nil, errors.WithStack(herodot.ErrUnauthorized.
			WithReason("The requested OAuth 2.0 client does not exist or you provided incorrect credentials.").WithDebug("Registration access tokens do not match."))
	}

	return c, nil
}

func (h *Handler) requireDynamicAuth(r *http.Request) *herodot.DefaultError {
	if !h.r.Config().PublicAllowDynamicRegistration(r.Context()) {
		return herodot.ErrNotFound.WithReason("Dynamic registration is not enabled.")
	}
	return nil
}
