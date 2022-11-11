// Copyright © 2022 Ory Corp
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"

	"github.com/spf13/cobra"

	hydra "github.com/ory/hydra-client-go/v2"
	"github.com/ory/hydra/cmd/cliclient"
	"github.com/ory/x/cmdx"
	"github.com/ory/x/flagx"
)

func NewCreateJWKSCmd() *cobra.Command {
	const alg = "alg"
	const use = "use"

	cmd := &cobra.Command{
		Use:     "jwk <set-id> [<key-id>]",
		Aliases: []string{"jwks"},
		Args:    cobra.RangeArgs(1, 2),
		Example: `{{ .CommandPath }} <my-jwk-set> --alg RS256 --use sig`,
		Short:   "Create a JSON Web Key Set with a JSON Web Key",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.CommandPath()
			m, _, err := cliclient.NewClient(cmd)
			if err != nil {
				return err
			}

			var kid string
			if len(args) == 2 {
				kid = args[1]
			}

			//nolint:bodyclose
			jwks, _, err := m.JwkApi.CreateJsonWebKeySet(context.Background(), args[0]).CreateJsonWebKeySet(hydra.CreateJsonWebKeySet{
				Alg: flagx.MustGetString(cmd, alg),
				Kid: kid,
				Use: flagx.MustGetString(cmd, use),
			}).Execute()
			if err != nil {
				return cmdx.PrintOpenAPIError(cmd, err)
			}

			cmdx.PrintTable(cmd, &outputJSONWebKeyCollection{Keys: jwks.Keys, Set: args[0]})
			return nil
		},
	}
	cmd.Root().Name()
	cmd.Flags().String(alg, "RS256", "The algorithm to be used to generated they key. Supports: RS256, RS512, ES256, ES512, EdDSA")
	cmd.Flags().String(use, "sig", "The intended use of this key. Supports: sig, enc")
	return cmd
}
