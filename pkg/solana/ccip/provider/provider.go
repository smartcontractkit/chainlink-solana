package provider

import (
	"context"
	"fmt"
	"sync"

	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	relaytypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/types/ccipocr3"

	chainsel "github.com/smartcontractkit/chain-selectors"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/ccip/chainaccessor"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/ccip/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/ccip/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/ccip/ocr"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"

	"github.com/smartcontractkit/libocr/offchainreporting2plus/ocr3types"
)

var _ commontypes.CCIPProvider = &Provider{}

const CCIPProviderName = "SolanaCCIPProvider"

type Provider struct {
	lggr  logger.Logger
	ca    ccipocr3.ChainAccessor
	ct    ocr3types.ContractTransmitter[[]byte]
	codec ccipocr3.Codec

	wg sync.WaitGroup
	services.StateMachine
}

func NewCCIPProvider(
	ctx context.Context,
	lggr logger.Logger,
	chainSelector ccipocr3.ChainSelector,
	pluginType ccipocr3.PluginType,
	client client.MultiClient,
	logPoller chainaccessor.AccessorLogPoller,
	fee fees.Estimator,
	txm txm.TxManager,
	ccipArgs relaytypes.CCIPProviderArgs,
) (*Provider, error) {
	foundSel := false
	for _, solChain := range chainsel.SolanaALL {
		if solChain.Selector == uint64(chainSelector) {
			foundSel = true
			break
		}
	}
	if !foundSel {
		return nil, fmt.Errorf("chain selector %d does not match any Solana chain selectors", chainSelector)
	}

	// Validate offramp address
	var offrampPubKey solana.PublicKey
	// NOTE: provider can still be initialized with an EVM offramp address, and PublicKeyFromBytes will panic on addresses with len=20
	// technically we only need the chainwriter to do fee estimation so this doesn't matter and we can use a zero address
	// TODO: Should we even build the rest of the provider? Or just initialize the accessor with just the estimator.
	if len(ccipArgs.OffRampAddress) == solana.PublicKeyLength {
		offrampPubKey = solana.PublicKeyFromBytes(ccipArgs.OffRampAddress)
	}

	c := ccipocr3.Codec{
		ChainSpecificAddressCodec: codec.NewAddressCodec(),
		CommitPluginCodec:         codec.NewCommitPluginCodecV1(),
		ExecutePluginCodec:        codec.NewExecutePluginCodecV1(ccipArgs.ExtraDataCodecBundle),
		TokenDataEncoder:          codec.NewSolanaTokenDataEncoder(),
		SourceChainExtraDataCodec: codec.NewExtraDataDecoder(),
		MessageHasher:             codec.NewMessageHasherV1(lggr, ccipArgs.ExtraDataCodecBundle),
	}

	ca, err := chainaccessor.NewSolanaAccessor(ctx, lggr, chainSelector, client, logPoller, fee, c.ChainSpecificAddressCodec)
	if err != nil {
		return nil, fmt.Errorf("failed to create Solana Chain Accessor: %w", err)
	}

	chainWriterConfig, err := config.GetSolanaChainWriterConfig(offrampPubKey.String(), string(ccipArgs.TransmitterAddress))
	if err != nil {
		return nil, fmt.Errorf("failed to build chain writer configs: %w", err)
	}

	cw, err := chainwriter.NewSolanaChainWriterService(lggr, client, txm, fee, chainWriterConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize chain writer: %w", err)
	}

	var ct ocr3types.ContractTransmitter[[]byte]
	switch pluginType {
	case ccipocr3.PluginTypeCCIPCommit:
		ct = ocr.NewCommitTransmitter(lggr, cw, string(ccipArgs.TransmitterAddress), offrampPubKey.String())
	case ccipocr3.PluginTypeCCIPExec:
		ct = ocr.NewExecTransmitter(lggr, cw, string(ccipArgs.TransmitterAddress), offrampPubKey.String(), ccipArgs.ExtraDataCodecBundle)
	default:
		return nil, fmt.Errorf("unsupported plugin type: %d", pluginType)
	}

	return &Provider{
		lggr:  logger.Named(lggr, CCIPProviderName),
		ca:    ca,
		ct:    ct,
		codec: c,
	}, nil
}

func (cp *Provider) Name() string {
	return cp.lggr.Name()
}

func (cp *Provider) Ready() error {
	return cp.StateMachine.Ready()
}

func (cp *Provider) Start(ctx context.Context) error {
	return cp.StartOnce(CCIPProviderName, func() error {
		cp.lggr.Debugw("Starting SolanaCCIPProvider")
		return nil
	})
}

func (cp *Provider) Close() error {
	return cp.StopOnce(CCIPProviderName, func() error {
		cp.wg.Wait()
		return nil
	})
}

func (cp *Provider) HealthReport() map[string]error {
	return map[string]error{cp.Name(): cp.Healthy()}
}

func (cp *Provider) ChainAccessor() ccipocr3.ChainAccessor {
	return cp.ca
}

func (cp *Provider) ContractTransmitter() ocr3types.ContractTransmitter[[]byte] {
	return cp.ct
}

func (cp *Provider) Codec() ccipocr3.Codec {
	return cp.codec
}
