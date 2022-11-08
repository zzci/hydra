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
	"context"
	"fmt"
	"os"

	hydra "github.com/ory/hydra-client-go"
	"github.com/ory/hydra/cmd/cliclient"
	"github.com/ory/x/cmdx"
	"github.com/ory/x/flagx"

	"github.com/spf13/cobra"
)

func NewRevokeTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "token the-token",
		Example: `{{ .CommandPath }} --client-id a0184d6c-b313-4e70-a0b9-905b581e9218 --client-secret Hh1BjioNNm ciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNT`,
		Args:    cobra.ExactArgs(1),
		Short:   "Revoke an access or refresh token",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := cliclient.NewClient(cmd)
			if err != nil {
				return err
			}

			clientID, clientSecret := flagx.MustGetString(cmd, "client-id"), flagx.MustGetString(cmd, "client-secret")
			if clientID == "" || clientSecret == "" {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), `%s

Please provide a Client ID and Client Secret using flags --client-id and --client-secret, or environment variables OAUTH2_CLIENT_ID and OAUTH2_CLIENT_SECRET
`, cmd.UsageString())
				return cmdx.FailSilently(cmd)
			}

			token := args[0]
			_, err = client.OAuth2Api.RevokeOAuth2Token(
				context.WithValue(cmd.Context(), hydra.ContextBasicAuth, hydra.BasicAuth{
					UserName: clientID,
					Password: clientSecret,
				})).Token(token).Execute() //nolint:bodyclose
			if err != nil {
				return cmdx.PrintOpenAPIError(cmd, err)
			}

			cmdx.PrintRow(cmd, cmdx.OutputIder(token))
			return nil
		},
	}
	cmd.Flags().String("client-id", os.Getenv("OAUTH2_CLIENT_ID"), "Use the provided OAuth 2.0 Client ID, defaults to environment variable OAUTH2_CLIENT_ID")
	cmd.Flags().String("client-secret", os.Getenv("OAUTH2_CLIENT_SECRET"), "Use the provided OAuth 2.0 Client Secret, defaults to environment variable OAUTH2_CLIENT_SECRET")
	return cmd
}
