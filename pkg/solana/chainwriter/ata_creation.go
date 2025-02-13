package chainwriter

import (
	"context"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/google/uuid"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

const (
	maxAtas = 12
)

func (s *SolanaChainWriterService) handleATACreation(ctx context.Context, createATAinstructions []solana.Instruction, methodConfig MethodConfig, contractName, method string, feePayer solana.PublicKey) error {
	blockhash, err := s.client.LatestBlockhash(ctx)
	if err != nil {
		return fmt.Errorf("error fetching latest blockhash: %w", err)
	}

	if len(createATAinstructions) > maxAtas {
		return fmt.Errorf("too many ATAs to create: %d, max allowed: %d", len(createATAinstructions), maxAtas)
	}
	ataTx, ataErr := solana.NewTransaction(
		createATAinstructions,
		blockhash.Value.Blockhash,
		solana.TransactionPayer(feePayer),
	)
	if ataErr != nil {
		return fmt.Errorf("error constructing ATA transaction: %w", err)
	}
	ataUUID := fmt.Sprintf("ATA-%s", uuid.NewString())

	s.lggr.Info("Sending create ATA transaction", "contract", contractName, "method", method)

	// Enqueue ATA transaction
	if err = s.txm.Enqueue(ctx, methodConfig.FromAddress, ataTx, &ataUUID, blockhash.Value.LastValidBlockHeight); err != nil {
		return fmt.Errorf("error enqueuing transaction: %w", err)
	}

	err = s.waitForTxFinality(ctx, ataUUID)
	if err != nil {
		return fmt.Errorf("error waiting for ATA transaction finality: %w", err)
	}
	return nil
}

func (s *SolanaChainWriterService) waitForTxFinality(ctx context.Context, transactionID string) error {
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("context ended while waiting for finality of transaction %s", transactionID)
		case <-ticker.C:
			status, err := s.txm.GetTransactionStatus(waitCtx, transactionID)
			if err != nil {
				return fmt.Errorf("error fetching transaction status: %w", err)
			}
			switch status {
			case types.Finalized:
				s.lggr.Debug("ATA transaction finalized", "transactionID", transactionID)
				return nil
			case types.Failed, types.Fatal:
				return fmt.Errorf("transaction %s failed", transactionID)
			default:
				// Keep polling
			}
		}
	}
}
