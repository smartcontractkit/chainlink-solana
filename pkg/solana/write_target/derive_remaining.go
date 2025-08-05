package writetarget

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	chainselectors "github.com/smartcontractkit/chain-selectors"
	"github.com/smartcontractkit/chainlink-common/pkg/capabilities"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/values"
	"github.com/smartcontractkit/chainlink-framework/capabilities/writetarget"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

var DeriveRemainingName = "derive_remaining"

func NewDeriveRemaining(chain Chain, client client.Reader, cfg config.Workflow, lggr logger.Logger) (capabilities.ExecutableCapability, error) {
	id := GenerateDeriveRemainingName(chain.ID())
	accs, err := accountsFromConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create derive remaining capability: %w", err)
	}

	capInfo := capabilities.MustNewCapabilityInfo(id, capabilities.CapabilityTypeAction, DeriveRemainingName)

	return &deriver{
		CapabilityInfo: capInfo,
		client:         client,
		accounts:       accs,
		lggr:           lggr,
	}, nil
}

func GenerateDeriveRemainingName(chainID string) string {
	id := fmt.Sprintf("derive_%v@1.0.0", chainID)

	chainName, err := chainselectors.SolanaNameFromChainId(chainID)
	if err == nil {
		wtID, err := writetarget.NewWriteTargetID("", chainName, chainID, "1.0.0")
		if err == nil {
			id = wtID
		}
	}

	return id
}

func NewWriteTargetID(chainFamilyName, networkName, chainID, version string) (string, error) {
	// Input args should not be empty
	if version == "" {
		return "", fmt.Errorf("version must not be empty")
	}

	// Network ID: network name is optional, if not provided, use the chain ID
	networkID := networkName
	if networkID == "" && chainID == "" {
		return "", fmt.Errorf("invalid input: networkName or chainID must not be empty")
	}
	if networkID == "" || networkID == "unknown" {
		networkID = chainID
	}

	// allow for chain family to be empty
	if chainFamilyName == "" {
		return fmt.Sprintf("%s_%s@%s", DeriveRemainingName, networkID, version), nil
	}

	return fmt.Sprintf("%s_%s-%s@%s", DeriveRemainingName, chainFamilyName, networkID, version), nil
}

type deriver struct {
	capabilities.CapabilityInfo
	client   client.Reader
	accounts accounts
	lggr     logger.Logger
}

type CacheDetails struct {
	State    string // State pubkey of df cache
	Receiver string // df cache programID
	FeedIDs  []string
}

func (dr *deriver) Execute(ctx context.Context, request capabilities.CapabilityRequest) (capabilities.CapabilityResponse, error) {
	var res capabilities.CapabilityResponse

	if request.Config == nil {
		return res, errors.New("missing config field")
	}

	var cd CacheDetails

	err := request.Config.UnwrapTo(&cd)
	if err != nil {
		return res, err
	}

	remaining, err := dr.deriveRemaining(ctx, &cd, request.Metadata)
	if err != nil {
		return res, err
	}

	res.Value, err = values.NewMap(map[string]solana.AccountMetaSlice{
		remainingAccountsKey: remaining,
	})

	if err != nil {
		return res, err
	}

	return res, nil
}

func (dr *deriver) deriveRemaining(ctx context.Context, cd *CacheDetails, meta capabilities.RequestMetadata) (solana.AccountMetaSlice, error) {
	var ret solana.AccountMetaSlice

	cacheProgram, err := solana.PublicKeyFromBase58(cd.Receiver)
	if err != nil {
		return nil, fmt.Errorf("can't parse cache programID: %w", err)
	}

	cacheStateKey, err := solana.PublicKeyFromBase58(cd.State)
	if err != nil {
		return nil, fmt.Errorf("failed parse cache state key: %w", err)
	}

	cacheStateAccount, err := dr.client.GetAccountInfoWithOpts(ctx, cacheStateKey, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentProcessed})
	if err != nil {
		return nil, fmt.Errorf("error fetching cache state account %v", cacheStateKey)
	}

	if cacheStateAccount.Value == nil {
		return nil, fmt.Errorf("cache state account does not exist %v", cacheStateKey)
	}

	// add cache state key
	ret = append(ret, &solana.AccountMeta{
		PublicKey:  cacheStateKey,
		IsWritable: false,
	})

	derivedAccounts := make([]*solana.AccountMeta, 2*len(cd.FeedIDs))

	authority, err := deriveForwarderAuthority(dr.accounts.forwarderState, cacheProgram, dr.accounts.forwarderProgramID)
	if err != nil {
		return nil, err
	}
	// derive pdas and check existence on-chain
	for i, feedID := range cd.FeedIDs {
		validBytes := validateBytes16(feedID)
		if !validBytes {
			return nil, fmt.Errorf("invalid feed id %v", feedID)
		}
		dataID, _ := new(big.Int).SetString(feedID, 0)
		var data [16]byte
		copy(data[:], dataID.Bytes())
		decimalReportSeeds := [][]byte{
			[]byte("decimal_report"),
			cacheStateKey.Bytes(),
			data[:],
		}

		decimalReportKey, _, err := solana.FindProgramAddress(decimalReportSeeds, cacheProgram)
		if err != nil {
			return nil, fmt.Errorf("could not derive decimal report PDA for data id %v: %w", feedID, err)
		}

		decimalReportAccount, err := dr.client.GetAccountInfoWithOpts(ctx, decimalReportKey, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentProcessed})
		if err != nil {
			return nil, fmt.Errorf("error fetching decimal report account %v for data id %v: %w", decimalReportKey, feedID, err)
		}

		if decimalReportAccount.Value == nil {
			return nil, fmt.Errorf("decimal report account %v does not exist for data id %v", decimalReportKey, feedID)
		}

		derivedAccounts[i] = &solana.AccountMeta{PublicKey: decimalReportKey, IsWritable: true}

		// add to remaining accounts
		reportHash := createReportHash(
			data[:],
			authority.Bytes(),
			[]byte(meta.WorkflowOwner),
			[]byte(meta.WorkflowName),
		)

		writeFlagSeeds := [][]byte{
			[]byte("permission_flag"),
			cacheStateKey.Bytes(),
			reportHash[:],
		}

		writeFlagKey, _, err := solana.FindProgramAddress(writeFlagSeeds, cacheProgram)
		if err != nil {
			return nil, fmt.Errorf("could not derive decimal report PDA for data id %v",
				feedID)
		}

		writeFlagAccount, err := dr.client.GetAccountInfoWithOpts(ctx, writeFlagKey, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentProcessed})
		if err != nil {
			return nil, fmt.Errorf("error fetching write flag account %v for data id %v", writeFlagKey, feedID)
		}

		if writeFlagAccount.Value == nil {
			return nil, fmt.Errorf("write flag account %v does not exist for data id %v", writeFlagKey, feedID)
		}

		// write flag accounts go after all the decimal report accounts
		derivedAccounts[len(cd.FeedIDs)+i] = &solana.AccountMeta{PublicKey: writeFlagKey, IsWritable: false}
	}

	ret = append(ret, derivedAccounts...)

	return ret, nil
}

func (dr *deriver) RegisterToWorkflow(ctx context.Context, request capabilities.RegisterToWorkflowRequest) error {
	// TODO: notify the background WriteTxConfirmer (workflow registered)
	return nil
}
func (dr *deriver) UnregisterFromWorkflow(ctx context.Context, request capabilities.UnregisterFromWorkflowRequest) error {
	// TODO: notify the background WriteTxConfirmer (workflow registered)
	return nil
}
