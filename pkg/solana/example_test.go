package solana_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExample(t *testing.T) {
	ctx := context.Background()

	cdc := makeTestCodec(t, fmt.Sprintf(baseIDL, uint64BaseTypeIDL, ""))
	onChainStruct := struct {
		I uint64
	}{
		I: 3,
	}

	bts, err := cdc.Encode(ctx, onChainStruct, "SimpleUint64Value")
	require.NoError(t, err)

	config := codec.ModifiersConfig{
		&codec.PropertyExtractorConfig{FieldName: "I"},
	}

	mod, err := config.ToModifier()
	require.NoError(t, err)

	mod, err = codec.NewByItemTypeModifier(map[string]codec.Modifier{"SimpleUint64Value": mod})
	require.NoError(t, err)

	modCodec, err := codec.NewModifierCodec(cdc, mod)
	require.NoError(t, err)

	_, err = modCodec.CreateType("SimpleUint64Value", true)
	require.NoError(t, err)

	var val uint64
	require.NoError(t, modCodec.Decode(ctx, bts, &val, "SimpleUint64Value"))

	assert.Equal(t, uint64(3), val)
}
