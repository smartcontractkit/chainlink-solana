package chainwriter

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/google/uuid"

	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/tokens"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
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

		mints, err := GetAddresses(ctx, args, []Lookup{lookup.MintAddress}, derivedTableMap, client)
		if lookup.Optional && isIgnorableError(err) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("error resolving mint address: %w", err)
		}

		for _, mint := range mints {
			accountInfo, err := client.GetAccountInfoWithOpts(ctx, mint.PublicKey, &rpc.GetAccountInfoOpts{
				Commitment: rpc.CommitmentFinalized,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to fetch account info for token mint %s: %w", mint.PublicKey.String(), err)
			}
			if accountInfo == nil || accountInfo.Value == nil {
				return nil, fmt.Errorf("failed to fetch account info for token mint %s", mint.PublicKey.String())
			}
			tokenProgram := accountInfo.Value.Owner

			ataAddress, _, err := tokens.FindAssociatedTokenAddress(tokenProgram, mint.PublicKey, wallet)
			if err != nil {
				return nil, fmt.Errorf("error deriving ATA: %w", err)
			}

			_, err = client.GetAccountInfoWithOpts(ctx, ataAddress, &rpc.GetAccountInfoOpts{
				Encoding:   "base64",
				Commitment: rpc.CommitmentFinalized,
			})
			if err == nil {
				logger.Debugw("ATA already exists, skipping creation.", "location", lookup.Location)
				continue
			}
			if !strings.Contains(err.Error(), "not found") {
				return nil, fmt.Errorf("error reading account info for ATA: %w", err)
			}

			ins, _, err := tokens.CreateAssociatedTokenAccount(tokenProgram, mint.PublicKey, wallet, feePayer)
			if err != nil {
				return nil, fmt.Errorf("error creating associated token account: %w", err)
			}
			createATAInstructions = append(createATAInstructions, ins)
		}
	}

	return createATAInstructions, nil
}

func (s *SolanaChainWriterService) handleATACreation(ctx context.Context, createATAinstructions []solana.Instruction, methodConfig MethodConfig, contractName, method string, feePayer solana.PublicKey) (string, error) {
	if len(createATAinstructions) == 0 {
		return "", nil
	}
	blockhash, err := s.client.LatestBlockhash(ctx)
	if err != nil {
		return "", fmt.Errorf("error fetching latest blockhash: %w", err)
	}

	if len(createATAinstructions) > maxAtas {
		return "", fmt.Errorf("too many ATAs to create: %d, max allowed: %d", len(createATAinstructions), maxAtas)
	}
	ataTx, ataErr := solana.NewTransaction(
		createATAinstructions,
		blockhash.Value.Blockhash,
		solana.TransactionPayer(feePayer),
	)
	if ataErr != nil {
		return "", fmt.Errorf("error constructing ATA transaction: %w", ataErr)
	}
	ataUUID := fmt.Sprintf("ATA-%s", uuid.NewString())

	s.lggr.Infow("Sending create ATA transaction", "contract", contractName, "method", method)

	// Enqueue ATA transaction
	if err = s.txm.Enqueue(ctx, methodConfig.FromAddress, ataTx, &ataUUID, blockhash.Value.LastValidBlockHeight, txmutils.SetEstimateComputeUnitLimit(true)); err != nil {
		return "", fmt.Errorf("error enqueuing transaction: %w", err)
	}

	return ataUUID, nil
}
