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

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ory/hydra/cmd/cliclient"
	"github.com/ory/x/cmdx"
	"github.com/ory/x/flagx"
	"github.com/ory/x/pointerx"

	"github.com/ory/hydra/cmd/cli"
)

const (
	flagClientAllowedCORSOrigin                 = "allowed-cors-origin"
	flagClientAudience                          = "audience"
	flagClientBackchannelLogoutCallback         = "backchannel-logout-callback"
	flagClientName                              = "name"
	flagClientClientURI                         = "client-uri"
	flagClientContact                           = "contact"
	flagClientFrontChannelLogoutSessionRequired = "frontchannel-logout-session-required"
	flagClientFrontChannelLogoutCallback        = "frontchannel-logout-callback"
	flagClientGrantType                         = "grant-type"
	flagClientJWKSURI                           = "jwks-uri"
	flagClientLogoURI                           = "logo-uri"
	flagClientMetadata                          = "metadata"
	flagClientOwner                             = "owner"
	flagClientPolicyURI                         = "policy-uri"
	flagClientPostLogoutCallback                = "post-logout-callback"
	flagClientRedirectURI                       = "redirect-uri"
	flagClientRequestObjectSigningAlg           = "request-object-signing-alg"
	flagClientRequestURI                        = "request-uri"
	flagClientResponseType                      = "response-type"
	flagClientScope                             = "scope"
	flagClientSectorIdentifierURI               = "sector-identifier-uri"
	flagClientSubjectType                       = "subject-type"
	flagClientTokenEndpointAuthMethod           = "token-endpoint-auth-method"
	flagClientSecret                            = "secret"
	flagClientTOSURI                            = "tos-uri"
	flagClientBackChannelLogoutSessionRequired  = "backchannel-logout-session-required"
)

func NewCreateClientsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "oauth2-client",
		Short:   "Create an OAuth 2.0 Client",
		Aliases: []string{"client"},
		Args:    cobra.NoArgs,
		Example: `{{ .CommandPath }} -n "my app" -c http://localhost/cb -g authorization_code -r code -a core,foobar

Use the tool jq (or any other JSON tool) to get the OAuth2 Client ID and and Secret:

client=$({{ .CommandPath }} \
    --format json \
    ...)
echo $client

# Parse the JSON response using jq to get the client ID and client secret:
client_id=$(echo $client | jq -r '.client_id')
client_secret=$(echo $client | jq -r '.client_secret')`,
		Long: `This command creates an OAuth 2.0 Client which can be used to perform various OAuth 2.0 Flows like
the Authorize Code, Implicit, Refresh flow. This command allows settings all fields defined in the OpenID Connect Dynamic Client Registration standard.

To encrypt an auto-generated OAuth2 Client Secret, use flags ` + "`--pgp-key`" + `, ` + "`--pgp-key-url`" + ` or ` + "`--keybase`" + ` flag, for example:

  {{ .CommandPath }} -n "my app" -g client_credentials -r token -a core,foobar --keybase keybase_username
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, _, err := cliclient.NewClient(cmd)
			if err != nil {
				return err
			}

			ek, encryptSecret, err := cli.NewEncryptionKey(cmd, nil)
			if err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Failed to load encryption key: %s", err)
				return err
			}

			secret := flagx.MustGetString(cmd, flagClientSecret)
			//nolint:bodyclose
			client, _, err := m.OAuth2Api.CreateOAuth2Client(cmd.Context()).OAuth2Client(clientFromFlags(cmd)).Execute()
			if err != nil {
				return cmdx.PrintOpenAPIError(cmd, err)
			}

			if client.ClientSecret == nil && len(secret) > 0 {
				client.ClientSecret = pointerx.String(secret)
			}

			if encryptSecret && client.ClientSecret != nil {
				enc, err := ek.Encrypt([]byte(*client.ClientSecret))
				if err != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Failed to encrypt client secret: %s", err)
					return cmdx.FailSilently(cmd)
				}

				client.ClientSecret = pointerx.String(enc.Base64Encode())
			}

			cmdx.PrintRow(cmd, (*outputOAuth2Client)(client))
			return nil
		},
	}
	registerClientFlags(cmd.Flags())
	return cmd
}
