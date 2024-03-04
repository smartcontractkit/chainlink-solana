package targets

import (
	"context"
	"fmt"

	sdk "github.com/gagliardetto/solana-go"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"

	"github.com/smartcontractkit/chainlink-common/pkg/capabilities"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/values"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
)

// func InitializeWrite(registry commontypes.CapabilitiesRegistry, lggr logger.Logger) error {
// 	for _, chain := range legacyEVMChains.Slice() {
// 		capability := NewSolanaWrite(chain, lggr)
// 		if err := registry.Add(context.TODO(), capability); err != nil {
// 			return err
// 		}
// 	}
// 	return nil
// }

var (
	_ capabilities.ActionCapability = &SolanaWrite{}
)

type SolanaWrite struct {
	chain  solana.Chain
	reader client.Reader
	capabilities.CapabilityInfo
	lggr logger.Logger
}

func NewSolanaWrite(chain solana.Chain, lggr logger.Logger) (*SolanaWrite, error) {
	// generate ID based on chain selector
	name := fmt.Sprintf("write_solana_%v", chain.ID())

	info := capabilities.MustNewCapabilityInfo(
		name,
		capabilities.CapabilityTypeTarget,
		"Write target.",
		"v1.0.0",
	)

	reader, err := chain.Reader()
	if err != nil {
		return nil, err
	}

	return &SolanaWrite{
		chain,
		reader,
		info,
		lggr,
	}, nil
}

type SolanaAccount struct {
	PublicKey  string `mapstructure:"public_key"`
	IsWritable bool   `mapstructure:"is_writable"`
	IsSigner   bool   `mapstructure:"is_signer"`
}

type SolanaConfig struct {
	ChainID           uint
	ReceiverProgramID sdk.PublicKey
	Params            []any
	Accounts          []SolanaAccount
	// ABI               string TODO:
}

// TODO: enforce required key presence

func parseConfig(rawConfig *values.Map) (SolanaConfig, error) {
	var config SolanaConfig
	configAny, err := rawConfig.Unwrap()
	if err != nil {
		return config, err
	}
	err = mapstructure.Decode(configAny, &config)
	return config, err
}

// func evaluateParams(params []any, inputs map[string]any) ([]any, error) {
// 	vars := pipeline.NewVarsFrom(inputs)
// 	var args []any
// 	for _, param := range params {
// 		switch v := param.(type) {
// 		case string:
// 			val, err := pipeline.VarExpr(v, vars)()
// 			if err == nil {
// 				args = append(args, val)
// 			} else if errors.Is(errors.Cause(err), pipeline.ErrParameterEmpty) {
// 				args = append(args, param)
// 			} else {
// 				return args, err
// 			}
// 		default:
// 			args = append(args, param)
// 		}
// 	}

// 	return args, nil
// }

func (cap *SolanaWrite) Execute(ctx context.Context, callback chan<- capabilities.CapabilityResponse, request capabilities.CapabilityRequest) error {
	cap.lggr.Debugw("Execute", "request", request)

	config := cap.chain.Config().ChainWriter()

	if config == nil {
		return fmt.Errorf("ChainWriter config undefined")
	}

	reqConfig, err := parseConfig(request.Config)
	if err != nil {
		return err
	}

	inputsAny, err := request.Inputs.Unwrap()
	if err != nil {
		return err
	}
	inputs := inputsAny.(map[string]any)

	blockhash, err := cap.reader.LatestBlockhash()

	programID := config.ForwarderProgramID
	state := config.ForwarderStateAccount
	authority := config.FromAddress

	// Determine store authority
	// TODO: compute this only once per capability init
	seeds := [][]byte{[]byte("forwarder"), state.Bytes()}
	forwarderAuthority, _, err := sdk.FindProgramAddress(seeds, programID)
	if err != nil {
		return errors.Wrap(err, "error on FindProgramAddress")
	}

	data := []byte{}

	// No signature validation in the MVP demo
	signatures := [][]byte{} // TODO: validate each sig is 64 bytes

	data = append(data, uint8(len(signatures))) // length prefix
	for _, sig := range signatures {
		data = append(data, sig...)
	}

	// TODO: encode inputs into data
	data = append(data, inputs["report"].([]byte)...)

	accounts := []*sdk.AccountMeta{
		// state
		{PublicKey: state, IsWritable: false, IsSigner: false},
		// authority (node's transmitter key that's the tx signer)
		{PublicKey: authority, IsWritable: false, IsSigner: true},
		// forwarder_authority
		{PublicKey: forwarderAuthority, IsWritable: false, IsSigner: false},
		// receiver_program
		{PublicKey: reqConfig.ReceiverProgramID, IsWritable: false, IsSigner: false},
		// ... rest passed from reqConfig
	}

	for _, account := range reqConfig.Accounts {
		publicKey, err := sdk.PublicKeyFromBase58(account.PublicKey)
		if err != nil {
			return err
		}
		accounts = append(accounts, &sdk.AccountMeta{
			PublicKey:  publicKey,
			IsWritable: account.IsWritable,
			IsSigner:   account.IsSigner,
		})
	}

	tx, err := sdk.NewTransaction(
		[]sdk.Instruction{
			sdk.NewInstruction(config.ForwarderProgramID, accounts, data),
		},
		blockhash.Value.Blockhash,
		sdk.TransactionPayer(authority),
	)
	if err != nil {
		return errors.Wrap(err, "error on Transmit.NewTransaction")
	}

	if err = cap.chain.TxManager().Enqueue(authority.String(), tx); err != nil {
		return err
	}

	go func() {
		// TODO: cast tx.Error to Err (or Value to Value?)
		callback <- capabilities.CapabilityResponse{
			Value: nil,
			Err:   nil,
		}
		close(callback)
	}()
	return nil
}

func (cap *SolanaWrite) RegisterToWorkflow(ctx context.Context, request capabilities.RegisterToWorkflowRequest) error {
	return nil
}

func (cap *SolanaWrite) UnregisterFromWorkflow(ctx context.Context, request capabilities.UnregisterFromWorkflowRequest) error {
	return nil
}
