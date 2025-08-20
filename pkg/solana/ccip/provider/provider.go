package provider

import (
	"context"
	"fmt"
	"sync"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/types/ccipocr3"

	chainsel "github.com/smartcontractkit/chain-selectors"

	ccipchainaccessor "github.com/smartcontractkit/chainlink-ccip/pkg/chainaccessor"
	"github.com/smartcontractkit/chainlink-ccip/pkg/contractreader"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/ccip/codec"

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

func NewCCIPProvider(lggr logger.Logger, chainSelector ccipocr3.ChainSelector, contractReader contractreader.Extended, contractWriter commontypes.ContractWriter) (*Provider, error) {
	if uint64(chainSelector) != chainsel.SOLANA_DEVNET.Selector && uint64(chainSelector) != chainsel.SOLANA_MAINNET.Selector {
		return nil, fmt.Errorf("unexpected chain selector: %d, expect either %d or %d", chainSelector, chainsel.SOLANA_DEVNET.Selector, chainsel.SOLANA_MAINNET.Selector)
	}

	ca, err := ccipchainaccessor.NewDefaultAccessor(logger.Named(lggr, "SolanaChainAccessor"), chainSelector, contractReader, contractWriter, codec.NewAddressCodec())
	if err != nil {
		return nil, fmt.Errorf("failed to create Solana Chain Accessor: %w", err)
	}

	return &Provider{
		lggr:  logger.Named(lggr, CCIPProviderName),
		ca:    ca,
		ct:    nil,              // unimplemented
		codec: ccipocr3.Codec{}, // unimplemented
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