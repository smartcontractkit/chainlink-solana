package fakes

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	solcap "github.com/smartcontractkit/chainlink-common/pkg/capabilities/v2/chain-capabilities/solana"
)

func TestConvertGetAccountInfoOpts(t *testing.T) {
	t.Parallel()

	t.Run("nil opts", func(t *testing.T) {
		t.Parallel()
		got, err := convertGetAccountInfoOpts(nil)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, solana.EncodingType(""), got.Encoding)
		assert.Equal(t, rpc.CommitmentType(""), got.Commitment)
		assert.Nil(t, got.DataSlice)
		assert.Nil(t, got.MinContextSlot)
	})

	t.Run("maps production defaults and slices", func(t *testing.T) {
		t.Parallel()
		got, err := convertGetAccountInfoOpts(&solcap.GetAccountInfoOpts{
			Encoding:       solcap.EncodingType_ENCODING_TYPE_JSON_PARSED,
			Commitment:     solcap.CommitmentType_COMMITMENT_TYPE_CONFIRMED,
			MinContextSlot: 42,
			DataSlice: &solcap.DataSlice{
				Offset: 3,
				Length: 9,
			},
		})
		require.NoError(t, err)
		require.NotNil(t, got)
		require.NotNil(t, got.DataSlice)
		require.NotNil(t, got.MinContextSlot)
		assert.Equal(t, solana.EncodingJSONParsed, got.Encoding)
		assert.Equal(t, rpc.CommitmentConfirmed, got.Commitment)
		assert.Equal(t, uint64(42), *got.MinContextSlot)
		assert.Equal(t, uint64(3), *got.DataSlice.Offset)
		assert.Equal(t, uint64(9), *got.DataSlice.Length)
	})
}

func TestConvertDataBytesOrJSON(t *testing.T) {
	t.Parallel()

	t.Run("raw binary encodings return raw bytes", func(t *testing.T) {
		t.Parallel()
		got, err := convertDataBytesOrJSON(rpc.DataBytesOrJSONFromBytes([]byte{0xca, 0xfe}), solana.EncodingBase64)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, solcap.EncodingType_ENCODING_TYPE_BASE64, got.GetEncoding())
		assert.Equal(t, []byte{0xca, 0xfe}, got.GetRaw())
		assert.Nil(t, got.GetJson())
	})

	t.Run("json parsed returns json payload", func(t *testing.T) {
		t.Parallel()
		var obj rpc.DataBytesOrJSON
		err := json.Unmarshal([]byte(`{"program":"spl-token","parsed":{"type":"account"}}`), &obj)
		require.NoError(t, err)

		got, err := convertDataBytesOrJSON(&obj, solana.EncodingJSONParsed)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, solcap.EncodingType_ENCODING_TYPE_JSON_PARSED, got.GetEncoding())
		assert.JSONEq(t, `{"program":"spl-token","parsed":{"type":"account"}}`, string(got.GetJson()))
		assert.Nil(t, got.GetRaw())
	})
}

func TestAccountToProto(t *testing.T) {
	t.Parallel()

	got, err := accountToProto(&rpc.Account{
		Lamports:   123,
		Owner:      solana.MustPublicKeyFromBase58("11111111111111111111111111111111"),
		Data:       rpc.DataBytesOrJSONFromBytes([]byte("hello")),
		Executable: true,
		RentEpoch:  big.NewInt(7),
		Space:      5,
	}, solana.EncodingBase64)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, uint64(123), got.GetLamports())
	assert.Equal(t, []byte("hello"), got.GetData().GetRaw())
	assert.Equal(t, uint64(5), got.GetSpace())
	assert.True(t, got.GetExecutable())
}
