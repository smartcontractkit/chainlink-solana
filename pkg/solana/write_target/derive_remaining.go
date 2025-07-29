package writetarget

import (
	"context"
	"errors"

	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink-common/pkg/capabilities"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
)

func NewDeriveRemaining() capabilities.ExecutableCapability {
	return &deriveRemaining{}
}

type deriveRemaining struct {
	capabilities.CapabilityInfo
	client client.Reader
}

var (
	RemainingAccs = "remaining_accounts"
)

type CacheDetails struct {
	Receiver string // programID of df cache program
	State    string // state of df cache
	FeedIds  []string
}

func (dr *deriveRemaining) Execute(ctx context.Context, request capabilities.CapabilityRequest) (capabilities.CapabilityResponse, error) {
	// execute derive remaining accounts logic
	// put into remainig_accounts field in
	var res capabilities.CapabilityResponse

	if request.Config == nil {
		return res, errors.New("missing config field")
	}

	var cd CacheDetails

	if err := request.Config.UnwrapTo(&cd); err != nil {
		return res, err
	}

	return res, nil
}

func (dr *deriveRemaining) getRemainingAccounts(ctx context.Context, cd *CacheDetails) ([]solana.AccountMeta, error) {
	return nil, nil
}

func (dr *deriveRemaining) RegisterToWorkflow(ctx context.Context, request capabilities.RegisterToWorkflowRequest) error {
	// TODO: notify the background WriteTxConfirmer (workflow registered)
	return nil
}
func (dr *deriveRemaining) UnregisterFromWorkflow(ctx context.Context, request capabilities.UnregisterFromWorkflowRequest) error {
	// TODO: notify the background WriteTxConfirmer (workflow registered)
	return nil
}
