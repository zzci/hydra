// Copyright © 2022 Ory Corp
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/ory/hydra/cmd/cliclient"
	"github.com/ory/x/cmdx"
)

func NewGetJWKSCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "jwk set-1 [set-2] ...",
		Aliases: []string{"jwks"},
		Args:    cobra.MinimumNArgs(1),
		Short:   "Get one or more JSON Web Key Set by its ID(s)",
		Long:    `This command gets all the details about an JSON Web Key. You can use this command in combination with jq.`,
		Example: `To get the JSON Web Key Set's secret, run:

	{{ .CommandPath }} <set-id> | jq -r '.[].use'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, _, err := cliclient.NewClient(cmd)
			if err != nil {
				return err
			}

			var sets outputJSONWebKeyCollection
			for _, set := range args {
				key, _, err := m.JwkApi.GetJsonWebKeySet(cmd.Context(), set).Execute() //nolint:bodyclose
				if err != nil {
					return cmdx.PrintOpenAPIError(cmd, err)
				}

				sets.Keys = append(sets.Keys, key.Keys...)
			}

			if len(sets.Keys) == 1 {
				cmdx.PrintRow(cmd, outputJsonWebKey{Set: args[0], JsonWebKey: sets.Keys[0]})
			} else if len(sets.Keys) > 1 {
				cmdx.PrintTable(cmd, sets)
			}

			return nil
		},
	}
}
