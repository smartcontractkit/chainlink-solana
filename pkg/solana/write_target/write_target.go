package writetarget

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	chainselectors "github.com/smartcontractkit/chain-selectors"

	"github.com/smartcontractkit/chainlink-common/pkg/capabilities"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-framework/capabilities/writetarget"
	"github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/report/platform/processor"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"

	ocr3types "github.com/smartcontractkit/chainlink-common/pkg/capabilities/consensus/ocr3/types"
)

type Chain interface {
	LatestHead(ctx context.Context) (commontypes.Head, error)
	//MultiClient() *client.MultiClient
	ID() string
	Config() config.Config
}

func New(chain Chain, reader client.Reader, txm Txm, chainInfo commontypes.ChainInfo, lggr logger.Logger) (capabilities.ExecutableCapability, error) {
	chainID := chain.ID()

	id := GenerateWriteTargetName(chainID)
	cfg := chain.Config().WF()

	emitter := writetarget.NewMonitorEmitter(lggr)

	processors, err := processor.NewPlatformProcessors(emitter)
	if err != nil {
		return nil, fmt.Errorf("failed to create solana platform processors: %w", err)
	}

	// TODO PLEX-297 implement products processors
	//processors["solana_secure_mint"] = secureMintProcessor

	beholder, err := writetarget.NewMonitor(writetarget.MonitorOpts{
		Lggr:              lggr,
		Processors:        processors,
		EnabledProcessors: processor.PlatformDefaultProcessors,
		Emitter:           emitter,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create solana WT monitor client: %+w", err)
	}

	ts, err := newTargetStrategy(reader, txm, cfg, lggr)
	if err != nil {
		return nil, fmt.Errorf("failed to create target strategy: %+w", err)
	}

	fromAddr := cfg.FromAddress()
	if fromAddr == nil {
		return nil, errors.New("missing from address")
	}
	fwdrAddr := cfg.ForwarderAddress()
	if fwdrAddr == nil {
		return nil, errors.New("missing forwarder address")
	}

	opts := writetarget.WriteTargetOpts{
		ID:     id,
		Logger: lggr,
		Config: writetarget.Config{
			PollPeriod:        cfg.PollPeriod(),
			AcceptanceTimeout: cfg.AcceptanceTimeout(),
		},
		ChainInfo:            chainInfo,
		Beholder:             beholder,
		ChainService:         chain,
		ConfigValidateFn:     evaluate,
		NodeAddress:          fromAddr.String(),
		ForwarderAddress:     fwdrAddr.String(),
		TargetStrategy:       ts,
		WriteAcceptanceState: *cfg.TxAcceptanceState(),
	}

	return writetarget.NewWriteTarget(opts), nil
}

func evaluate(rawRequest capabilities.CapabilityRequest) (string, error) {
	r, err := getRequest(rawRequest)
	if err != nil {
		return "", err
	}

	// don't need tail in this case
	reportMetadata, _, err := ocr3types.Decode(r.Inputs.SignedReport.Report)
	if err != nil {
		return "", fmt.Errorf("failed to decode report metadata: %w", err)
	}

	if reportMetadata.Version != 1 {
		return "", fmt.Errorf("unsupported report version: %d", reportMetadata.Version)
	}

	if reportMetadata.ExecutionID != rawRequest.Metadata.WorkflowExecutionID {
		return "", fmt.Errorf("WorkflowExecutionID in the report does not match WorkflowExecutionID in the request metadata. Report WorkflowExecutionID: %+v, request WorkflowExecutionID: %+v", hex.EncodeToString([]byte(reportMetadata.ExecutionID)), rawRequest.Metadata.WorkflowExecutionID)
	}

	// case-insensitive verification of the owner address (so that a check-summed address matches its non-checksummed version).
	if !strings.EqualFold(reportMetadata.WorkflowOwner, rawRequest.Metadata.WorkflowOwner) {
		return "", fmt.Errorf("WorkflowOwner in the report does not match WorkflowOwner in the request metadata. Report WorkflowOwner: %+v, request WorkflowOwner: %+v", reportMetadata.WorkflowOwner, rawRequest.Metadata.WorkflowOwner)
	}

	if !strings.EqualFold(reportMetadata.WorkflowName, rawRequest.Metadata.WorkflowName) {
		return "", fmt.Errorf("WorkflowName in the report does not match WorkflowName in the request metadata. Report WorkflowName: %+v, request WorkflowName: %+v", reportMetadata.WorkflowName, rawRequest.Metadata.WorkflowName)
	}

	if reportMetadata.WorkflowID != rawRequest.Metadata.WorkflowID {
		return "", fmt.Errorf("WorkflowID in the report does not match WorkflowID in the request metadata. Report WorkflowID: %+v, request WorkflowID: %+v", reportMetadata.WorkflowID, rawRequest.Metadata.WorkflowID)
	}

	byteID, err := hex.DecodeString(reportMetadata.ReportID)
	if err != nil {
		return "", fmt.Errorf("failed to decode report ID: %w", err)
	}

	if !bytes.Equal(byteID, r.Inputs.SignedReport.ID) {
		return "", fmt.Errorf("ReportID in the report does not match ReportID in the inputs. reportMetadata.ReportID: %x, Inputs.SignedReport.ID: %x", reportMetadata.ReportID, r.Inputs.SignedReport.ID)
	}

	return r.Config.Address, nil
}

func GenerateWriteTargetName(chainID string) string {
	id := fmt.Sprintf("write_%v@1.0.0", strings.ToLower(chainID))

	chainName, err := chainselectors.SolanaNameFromChainId(chainID)
	if err == nil {
		wtID, err := writetarget.NewWriteTargetID("", chainName, chainID, "1.0.0")
		if err == nil {
			id = wtID
		}
	}

	return id
}
