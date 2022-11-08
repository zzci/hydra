package cmd_test

import (
	"context"
	"encoding/base64"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gofrs/uuid"

	"gopkg.in/square/go-jose.v2"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/ory/hydra/client"
	"github.com/ory/hydra/driver"
	"github.com/ory/hydra/internal"
	"github.com/ory/hydra/internal/testhelpers"
	"github.com/ory/x/cmdx"
	"github.com/ory/x/contextx"
	"github.com/ory/x/snapshotx"
)

func base64EncodedPGPPublicKey(t *testing.T) string {
	t.Helper()
	gpgPublicKey, err := os.ReadFile("../test/stub/pgp.pub")
	if err != nil {
		t.Fatal(err)
	}

	return base64.StdEncoding.EncodeToString(gpgPublicKey)
}

func setupRoutes(t *testing.T, cmd *cobra.Command) (*httptest.Server, *httptest.Server, driver.Registry) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	reg := internal.NewMockedRegistry(t, &contextx.Default{})
	public, admin := testhelpers.NewOAuth2Server(ctx, t, reg)

	cmdx.RegisterHTTPClientFlags(cmd.Flags())
	cmdx.RegisterFormatFlags(cmd.Flags())
	require.NoError(t, cmd.Flags().Set(cmdx.FlagFormat, string(cmdx.FormatJSON)))
	return public, admin, reg
}

func setup(t *testing.T, cmd *cobra.Command) driver.Registry {
	_, admin, reg := setupRoutes(t, cmd)
	require.NoError(t, cmd.Flags().Set(cmdx.FlagEndpoint, admin.URL))
	require.NoError(t, cmd.Flags().Set(cmdx.FlagFormat, string(cmdx.FormatJSON)))
	return reg
}

var snapshotExcludedClientFields = []snapshotx.ExceptOpt{
	snapshotx.ExceptNestedKeys("client_id"),
	snapshotx.ExceptNestedKeys("registration_access_token"),
	snapshotx.ExceptNestedKeys("registration_client_uri"),
	snapshotx.ExceptNestedKeys("client_secret"),
	snapshotx.ExceptNestedKeys("created_at"),
	snapshotx.ExceptNestedKeys("updated_at"),
}

func createClientCredentialsClient(t *testing.T, reg driver.Registry) *client.Client {
	return createClient(t, reg, &client.Client{
		GrantTypes:              []string{"client_credentials"},
		TokenEndpointAuthMethod: "client_secret_basic",
		Secret:                  uuid.Must(uuid.NewV4()).String(),
	})
}

func createClient(t *testing.T, reg driver.Registry, c *client.Client) *client.Client {
	if c == nil {
		c = &client.Client{TokenEndpointAuthMethod: "client_secret_post", Secret: uuid.Must(uuid.NewV4()).String()}
	}
	secret := c.Secret
	require.NoError(t, reg.ClientManager().CreateClient(context.Background(), c))
	c.Secret = secret
	return c
}

func createJWK(t *testing.T, reg driver.Registry, set string, alg string) jose.JSONWebKey {
	c, err := reg.KeyManager().GenerateAndPersistKeySet(context.Background(), set, "", alg, "sig")
	require.NoError(t, err)
	return c.Keys[0]
}
