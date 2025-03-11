package chainwriter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/google/uuid"

	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/tokens"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/types"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
)

const (
	maxAtas = 12
)

// CreateATAs first checks if a specified location exists, then checks if the accounts derived from the
// ATALookups in the ChainWriter's configuration exist on-chain and creates them if they do not.
func CreateATAs(ctx context.Context, args any, lookups []ATALookup, derivedTableMap map[string]map[string][]*solana.AccountMeta, client client.MultiClient, feePayer solana.PublicKey, logger logger.Logger) ([]solana.Instruction, error) {
	createATAInstructions := []solana.Instruction{}
	for _, lookup := range lookups {
		// Check if location exists
		if lookup.Location != "" {
			_, err := GetValuesAtLocation(args, lookup.Location)
			if err != nil {
				// field doesn't exist, so ignore ATA creation
				if errors.Is(err, errFieldNotFound) {
					logger.Debugw("field not found, skipping ATA creation", "location", lookup.Location)
					continue
				}
				return nil, fmt.Errorf("error getting values at location: %w", err)
			}
		}
		walletAddresses, err := GetAddresses(ctx, args, []Lookup{lookup.WalletAddress}, derivedTableMap, client)
		if lookup.Optional && isIgnorableError(err) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("error resolving wallet address: %w", err)
		}
		if len(walletAddresses) != 1 {
			return nil, fmt.Errorf("expected exactly one wallet address, got %d", len(walletAddresses))
		}
		wallet := walletAddresses[0].PublicKey

		tokenPrograms, err := GetAddresses(ctx, args, []Lookup{lookup.TokenProgram}, derivedTableMap, client)
		if lookup.Optional && isIgnorableError(err) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("error resolving token program address: %w", err)
		}

		mints, err := GetAddresses(ctx, args, []Lookup{lookup.MintAddress}, derivedTableMap, client)
		if lookup.Optional && isIgnorableError(err) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("error resolving mint address: %w", err)
		}
		if len(tokenPrograms) != len(mints) {
			return nil, fmt.Errorf("expected equal number of token programs and mints, got %d tokenPrograms and %d mints", len(tokenPrograms), len(mints))
		}

		for i := range tokenPrograms {
			tokenProgram := tokenPrograms[i].PublicKey
			mint := mints[i].PublicKey

			ataAddress, _, err := tokens.FindAssociatedTokenAddress(tokenProgram, mint, wallet)
			if err != nil {
				return nil, fmt.Errorf("error deriving ATA: %w", err)
			}

			_, err = client.GetAccountInfoWithOpts(ctx, ataAddress, &rpc.GetAccountInfoOpts{
				Encoding:   "base64",
				Commitment: rpc.CommitmentFinalized,
			})
			if err == nil {
				logger.Infow("ATA already exists, skipping creation.", "location", lookup.Location)
				continue
			}
			if !strings.Contains(err.Error(), "not found") {
				return nil, fmt.Errorf("error reading account info for ATA: %w", err)
			}

			ins, _, err := tokens.CreateAssociatedTokenAccount(tokenProgram, mint, wallet, feePayer)
			if err != nil {
				return nil, fmt.Errorf("error creating associated token account: %w", err)
			}
			createATAInstructions = append(createATAInstructions, ins)
		}
	}

	return createATAInstructions, nil
}

func (s *SolanaChainWriterService) handleATACreation(ctx context.Context, createATAinstructions []solana.Instruction, methodConfig MethodConfig, contractName, method string, feePayer solana.PublicKey) error {
	if len(createATAinstructions) == 0 {
		return nil
	}
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

	s.lggr.Infow("Sending create ATA transaction", "contract", contractName, "method", method)

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
