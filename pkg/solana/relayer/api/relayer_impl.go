package api

import (
	"context"
	"time"

	solanago "github.com/gagliardetto/solana-go"
	"github.com/google/uuid"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
)

type solanaAPIImpl struct {
	chain solana.Chain
}

func (api solanaAPIImpl) SubmitTransaction(ctx context.Context, request TransactionRequest) (string, error) {
	blockhash, err := api.chain.MultiClient().LatestBlockhash(ctx)
	if err != nil {
		return "", err
	}
	var txID *string = ptr(uuid.NewString()) //seems that new txs require nil
	options := []txmutils.SetTxConfig{}

	tx, err := solanago.NewTransaction(
		request.Instructions,
		blockhash.Value.Blockhash,
		solanago.TransactionPayer(request.Payer),
		solanago.TransactionAddressTables(map[solanago.PublicKey]solanago.PublicKeySlice{}),
	)
	if err != nil {
		return "", err
	}

	err = api.chain.TxManager().Enqueue(ctx, request.Payer.String(), tx, txID, blockhash.Value.LastValidBlockHeight, options...)
	if err != nil {
		return "", err
	}

	return *txID, nil
}

func (api solanaAPIImpl) GetTransactionStatus(ctx context.Context, txID string) (TxState, error) {
	state, err := api.chain.TxManager().GetTxStatus(ctx, txID)
	if err != nil {
		return Errored, err
	}

	switch state {
	case txmutils.Errored:
		return Errored, nil
	case txmutils.AwaitingBroadcast:
		return AwaitingBroadcast, nil
	case txmutils.Broadcasted:
		return Broadcasted, nil
	case txmutils.Processed:
		return Processed, nil
	case txmutils.Confirmed:
		return Confirmed, nil
	case txmutils.Finalized:
		return Finalized, nil
	case txmutils.FatallyErrored:
		return FatallyErrored, nil
	default:
		return NotFound, nil
	}
}

func (api solanaAPIImpl) WaitUntilState(ctx context.Context, request WaitUntilStateRequest) error {
	var duration = time.Second * 5
	if request.Duration != nil {
		duration = *request.Duration
	}
	initialTime := time.Now()
	for {
		state, err := api.GetTransactionStatus(ctx, request.TxID)
		if err != nil {
			return err
		}
		if state == request.TxState {
			return nil
		}
		spentTime := time.Now().Sub(initialTime)
		if duration.Abs().Milliseconds() <= spentTime.Milliseconds() {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func ptr(s string) *string {
	return &s
}

func NewSolanaRelayerAPI(ctx context.Context, config SolanaRelayerConfig, providedAPIs *RequiredAPIs) (SolanaRelayerAPI, error) {
	chain, err := solana.NewChain(&config.TOMLConfig, solana.ChainOpts{Logger: *config.Logger, KeyStore: providedAPIs.KeystoreAPI})

	if err != nil {
		return nil, err
	}
	err = chain.Start(ctx)
	if err != nil {
		return nil, err
	}
	return solanaAPIImpl{
		chain: chain,
	}, nil
}