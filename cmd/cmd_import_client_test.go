package cmd_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	hydra "github.com/ory/hydra-client-go"
	"github.com/ory/hydra/cmd"
	"github.com/ory/x/cmdx"
	"github.com/ory/x/pointerx"
	"github.com/ory/x/snapshotx"
)

func writeTempFile(t *testing.T, contents interface{}) string {
	t.Helper()
	ij, err := json.Marshal(contents)
	require.NoError(t, err)
	f, err := os.CreateTemp(t.TempDir(), "")
	require.NoError(t, err)
	_, err = f.Write(ij)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

func TestImportClient(t *testing.T) {
	ctx := context.Background()
	c := cmd.NewImportClientCmd()
	reg := setup(t, c)

	file1 := writeTempFile(t, []hydra.OAuth2Client{{Scope: pointerx.String("foo")}, {Scope: pointerx.String("bar"), ClientSecret: pointerx.String("some-secret")}})
	file2 := writeTempFile(t, []hydra.OAuth2Client{{Scope: pointerx.String("baz")}, {Scope: pointerx.String("zab"), ClientSecret: pointerx.String("some-secret")}})

	t.Run("case=imports clients from single file", func(t *testing.T) {
		actual := gjson.Parse(cmdx.ExecNoErr(t, c, file1))
		require.Len(t, actual.Array(), 2)
		assert.NotEmpty(t, actual.Get("0.client_id").String())
		assert.NotEmpty(t, actual.Get("0.client_secret").String())
		assert.Equal(t, "some-secret", actual.Get("1.client_secret").String())

		_, err := reg.ClientManager().GetClient(ctx, actual.Get("0.client_id").String())
		require.NoError(t, err)

		_, err = reg.ClientManager().GetClient(ctx, actual.Get("1.client_id").String())
		require.NoError(t, err)

		snapshotx.SnapshotT(t, json.RawMessage(actual.Raw), snapshotExcludedClientFields...)
	})

	t.Run("case=imports clients from multiple files", func(t *testing.T) {
		actual := gjson.Parse(cmdx.ExecNoErr(t, c, file1, file2))
		require.Len(t, actual.Array(), 4)

		for _, v := range []string{
			"foo", "bar", "baz", "zab",
		} {
			found := false
			for _, j := range actual.Array() {
				if j.Get("scope").String() == v {
					found = true
					break
				}
			}
			assert.True(t, found, "missing client with scope %s", v)
		}
	})

	t.Run("case=imports clients from multiple files and stdin", func(t *testing.T) {
		var stdin bytes.Buffer
		require.NoError(t, json.NewEncoder(&stdin).Encode([]hydra.OAuth2Client{{Scope: pointerx.String("oof")}, {Scope: pointerx.String("rab"), ClientSecret: pointerx.String("some-secret")}}))

		stdout, _, err := cmdx.Exec(t, c, &stdin, file1, file2)
		require.NoError(t, err)
		actual := gjson.Parse(stdout)
		require.Len(t, actual.Array(), 6)
		var found bool
		for _, v := range actual.Array() {
			if v.Get("scope").String() == "oof" {
				found = true
			}
		}
		assert.True(t, found)
	})

	t.Run("case=performs appropriate error reporting", func(t *testing.T) {
		file3 := writeTempFile(t, []hydra.OAuth2Client{{ClientSecret: pointerx.String("short")}})
		stdout, stderr, err := cmdx.Exec(t, c, nil, file1, file3)
		require.Error(t, err)
		actual := gjson.Parse(stdout)
		require.Len(t, actual.Array(), 2)
		assert.Contains(t, stderr, "secret that is at least 6 characters long")

		snapshotx.SnapshotT(t, json.RawMessage(actual.Raw), snapshotExcludedClientFields...)
	})
}
