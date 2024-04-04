package monitoring

import (
	"fmt"
	"testing"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-common/pkg/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/testutils"
)

func TestTxResultsSource(t *testing.T) {
	cr := mocks.NewChainReader(t)
	lgr := logger.Test(t)
	ctx := utils.Context(t)

	factory := NewTxResultsSourceFactory(cr, lgr)
	assert.Equal(t, txresultsType, factory.GetType())

	// generate source
	source, err := factory.NewSource(nil, nil)
	assert.Error(t, err)
	source, err = factory.NewSource(nil, config.SolanaFeedConfig{
		StateAccount: testutils.GeneratePublicKey(),
	})

	success, fail, sigs := testutils.GenerateTransactionSignatures()
	assert.Equal(t, 100, success+fail)
	cr.On("GetSignaturesForAddressWithOpts", mock.Anything, mock.Anything, mock.Anything).Return([]*rpc.TransactionSignature{}, fmt.Errorf("fail")).Once()
	cr.On("GetSignaturesForAddressWithOpts", mock.Anything, mock.Anything, mock.Anything).Return(sigs, nil).Once()

	// fail on get signatures
	_, err = source.Fetch(ctx)
	assert.ErrorContains(t, err, "failed to fetch transactions for state account")

	// happy path
	out, err := source.Fetch(ctx)
	require.NoError(t, err)
	counts, ok := out.(commonMonitoring.TxResults)
	require.True(t, ok)

	// validate counts
	assert.Equal(t, success, int(counts.NumSucceeded))
	assert.Equal(t, fail, int(counts.NumFailed))
}
