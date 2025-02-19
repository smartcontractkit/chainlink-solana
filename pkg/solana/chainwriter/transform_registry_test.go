package chainwriter_test

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink-ccip/pkg/types/ccipocr3"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
)

func Test_CCIPCommitTransform(t *testing.T) {
	ctx := tests.Context(t)
	offrampAddress, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	key1, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	key2, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	t.Run("ArgsTransform does not affect accounts if token prices exist", func(t *testing.T) {
		args := struct {
			Info ccipocr3.CommitReportInfo
		}{
			Info: ccipocr3.CommitReportInfo{
				TokenPrices: []ccipocr3.TokenPrice{{TokenID: ccipocr3.UnknownEncodedAddress(key1.PublicKey().String())}},
			},
		}
		accounts := []*solana.AccountMeta{
			{
				PublicKey: key1.PublicKey(),
			},
			{
				PublicKey: key2.PublicKey(),
			},
		}
		_, newAccounts, err := chainwriter.CCIPCommitAccountTransform(ctx, nil, args, accounts, offrampAddress.PublicKey().String())
		require.NoError(t, err)
		require.Len(t, newAccounts, 2)
	})
	t.Run("ArgsTransform removes last account if token and gas prices do not exist", func(t *testing.T) {
		args := struct {
			Info ccipocr3.CommitReportInfo
		}{
			Info: ccipocr3.CommitReportInfo{},
		}
		accounts := []*solana.AccountMeta{
			{
				PublicKey: key1.PublicKey(),
			},
			{
				PublicKey: key2.PublicKey(),
			},
		}
		_, newAccounts, err := chainwriter.CCIPCommitAccountTransform(ctx, nil, args, accounts, offrampAddress.PublicKey().String())
		require.NoError(t, err)
		require.Len(t, newAccounts, 1)
	})
}
