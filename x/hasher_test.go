package x

import (
	"context"
	"fmt"
	"testing"

	"github.com/ory/x/hasherx"

	"github.com/stretchr/testify/require"
)

type hasherConfig struct {
	cost uint32
}

func (c hasherConfig) HasherPBKDF2Config(ctx context.Context) *hasherx.PBKDF2Config {
	return &hasherx.PBKDF2Config{}
}

func (c hasherConfig) HasherBcryptConfig(ctx context.Context) *hasherx.BCryptConfig {
	return &hasherx.BCryptConfig{Cost: c.cost}
}

func (c hasherConfig) GetHasherAlgorithm(ctx context.Context) HashAlgorithm {
	return HashAlgorithmPBKDF2
}

func TestHasher(t *testing.T) {
	for _, cost := range []uint32{1, 8, 10} {
		result, err := NewHasher(&hasherConfig{cost: cost}).Hash(context.Background(), []byte("foobar"))
		require.NoError(t, err)
		require.NotEmpty(t, result)
	}
}

// TestBackwardsCompatibility confirms that hashes generated with v1.x work with v2.x.
func TestBackwardsCompatibility(t *testing.T) {
	h := NewHasher(new(hasherConfig))
	require.NoError(t, h.Compare(context.Background(), []byte("$2a$10$lsrJjLPOUF7I75s3339R2uwqpjSlYGfhFyg7YsPtrSoITVy5UF3B2"), []byte("secret")))
	require.NoError(t, h.Compare(context.Background(), []byte("$2a$10$O1jZhd3U0azpLXwTu0cHHuTDWsBFnTJVbeHTADNQJWPR4Zqs8ATKS"), []byte("secret")))
	require.Error(t, h.Compare(context.Background(), []byte("$2a$10$lsrJjLPOUF7I75s3339R2uwqpjSlYGfhFyg7YsPtrSoITVy5UF3B3"), []byte("secret")))
}

func BenchmarkHasher(b *testing.B) {
	for cost := uint32(1); cost <= 16; cost++ {
		b.Run(fmt.Sprintf("cost=%d", cost), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				result, err := NewHasher(&hasherConfig{cost: cost}).Hash(context.Background(), []byte("foobar"))
				require.NoError(b, err)
				require.NotEmpty(b, result)
			}
		})
	}
}
