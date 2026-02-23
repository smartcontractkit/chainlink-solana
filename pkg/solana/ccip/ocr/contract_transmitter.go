package ocr

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"

	"github.com/google/uuid"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/types/ccip/consts"
	"github.com/smartcontractkit/chainlink-common/pkg/types/ccipocr3"

	"github.com/smartcontractkit/libocr/offchainreporting2plus/chains/evmutil"
	"github.com/smartcontractkit/libocr/offchainreporting2plus/ocr3types"
	ocrtypes "github.com/smartcontractkit/libocr/offchainreporting2plus/types"
)

// ToCalldataFunc is a function that takes in the OCR3 report and signature data and processes them.
// It returns the contract name, method name, and arguments for the on-chain contract call.
// The ReportWithInfo bytes field is also decoded according to the implementation of this function,
// the commit and execute plugins have different representations for this data.
type ToCalldataFunc func(
	rawReportCtx [2][32]byte,
	report ocr3types.ReportWithInfo[[]byte],
	rs, ss [][32]byte,
	vs [32]byte,
	codec ccipocr3.ExtraDataCodecBundle,
) (contract string, method string, args any, err error)

// TODO: Update type in chainlink-ccip and directly reference those
// https://github.com/smartcontractkit/chainlink-ccip/blob/main/chains/solana/types.go
// commitCallArgs defines the calldata structure for an SVM commit transaction.
// IMPORTANT: The names and types of the fields are critical because the chainwriter uses mapstructure
// to map these fields to the contract's parameter names. Changing these names or types (or omitting the
// mapstructure tags) may result in transactions being constructed with incorrect arguments.
type commitCallArgs struct {
	ReportContext [2][32]byte               `mapstructure:"ReportContext"`
	Report        []byte                    `mapstructure:"Report"`
	Rs            [][32]byte                `mapstructure:"Rs"`
	Ss            [][32]byte                `mapstructure:"Ss"`
	RawVs         [32]byte                  `mapstructure:"RawVs"`
	Info          ccipocr3.CommitReportInfo `mapstructure:"Info"`
}

// TODO: Update type in chainlink-ccip and directly reference those
// https://github.com/smartcontractkit/chainlink-ccip/blob/main/chains/solana/types.go
// execCallArgs defines the calldata structure for an SVM execute transaction.
// IMPORTANT: The names and types of the fields are critical because the chainwriter uses mapstructure
// to map these fields to the contract's parameter names. Changing these names or types (or omitting the
// mapstructure tags) may result in transactions being constructed with incorrect arguments.
type ExecCallArgs struct {
	ReportContext [2][32]byte                `mapstructure:"ReportContext"`
	Report        []byte                     `mapstructure:"Report"`
	Info          ccipocr3.ExecuteReportInfo `mapstructure:"Info"`
	ExtraData     ccipocr3.ExtraDataDecoded  `mapstructure:"ExtraData"`
	TokenIndexes  []byte                     `mapstructure:"TokenIndexes"`
}

var _ ocr3types.ContractTransmitter[[]byte] = &ccipTransmitter{}

type ccipTransmitter struct {
	cw             commontypes.ContractWriter
	fromAccount    string
	offrampAddress string
	toCalldataFn   ToCalldataFunc
	extraDataCodec ccipocr3.ExtraDataCodecBundle
	lggr           logger.Logger
}

// NewCommitTransmitter constructs a commit transmitter.
func NewCommitTransmitter(
	lggr logger.Logger,
	cw types.ContractWriter,
	fromAccount string,
	offrampAddress string,
) ocr3types.ContractTransmitter[[]byte] {
	return &ccipTransmitter{
		lggr:           logger.Named(lggr, "SolanaCommitTransmitter"),
		cw:             cw,
		fromAccount:    fromAccount,
		offrampAddress: offrampAddress,
		toCalldataFn:   commitCalldataFunc,
	}
}

// NewExecTransmitter constructs an execute transmitter.
func NewExecTransmitter(
	lggr logger.Logger,
	cw types.ContractWriter,
	fromAccount string,
	offrampAddress string,
	extraDataCodec ccipocr3.ExtraDataCodecBundle,
) ocr3types.ContractTransmitter[[]byte] {
	return &ccipTransmitter{
		lggr:           logger.Named(lggr, "SolanaExecTransmitter"),
		cw:             cw,
		fromAccount:    fromAccount,
		offrampAddress: offrampAddress,
		toCalldataFn:   execCalldataFunc,
		extraDataCodec: extraDataCodec,
	}
}

func XXXNewContractTransmitterTestsOnly(
	lggr logger.Logger,
	cw commontypes.ContractWriter,
	fromAccount string,
	contractName string,
	method string,
	offrampAddress string,
	toCalldataFn ToCalldataFunc,
) ocr3types.ContractTransmitter[[]byte] {
	wrappedToCalldataFunc := func(rawReportCtx [2][32]byte,
		report ocr3types.ReportWithInfo[[]byte],
		rs, ss [][32]byte,
		vs [32]byte,
		extraDataCodec ccipocr3.ExtraDataCodecBundle,
	) (string, string, any, error) {
		_, _, args, err := toCalldataFn(rawReportCtx, report, rs, ss, vs, extraDataCodec)
		return contractName, method, args, err
	}
	return &ccipTransmitter{
		lggr:           lggr,
		cw:             cw,
		fromAccount:    fromAccount,
		offrampAddress: offrampAddress,
		toCalldataFn:   wrappedToCalldataFunc,
	}
}

// FromAccount implements ocr3types.ContractTransmitter.
func (c *ccipTransmitter) FromAccount(context.Context) (ocrtypes.Account, error) {
	return ocrtypes.Account(c.fromAccount), nil
}

// Transmit implements ocr3types.ContractTransmitter.
func (c *ccipTransmitter) Transmit(
	ctx context.Context,
	configDigest ocrtypes.ConfigDigest,
	seqNr uint64,
	reportWithInfo ocr3types.ReportWithInfo[[]byte],
	sigs []ocrtypes.AttributedOnchainSignature,
) error {
	if len(sigs) > 32 {
		return errors.New("too many signatures, maximum is 32")
	}

	// report ctx for OCR3 consists of the following
	// reportContext[0]: ConfigDigest
	// reportContext[1]: 24 byte padding, 8 byte sequence number
	rawReportCtx := rawReportContext3(configDigest, seqNr)

	var contract string
	var method string
	var args any
	var err error

	var rs [][32]byte
	var ss [][32]byte
	var vs [32]byte
	for i, as := range sigs {
		r, s, v, err2 := evmutil.SplitSignature(as.Signature)
		if err2 != nil {
			return fmt.Errorf("failed to split signature: %w", err)
		}
		rs = append(rs, r)
		ss = append(ss, s)
		vs[i] = v
	}

	// chain writer takes in the raw calldata and packs it on its own.
	contract, method, args, err = c.toCalldataFn(rawReportCtx, reportWithInfo, rs, ss, vs, c.extraDataCodec)
	if err != nil {
		return fmt.Errorf("failed to generate ecdsa call data: %w", err)
	}

	// TODO: no meta fields yet, what should we add?
	// probably whats in the info part of the report?
	meta := commontypes.TxMeta{}
	txUUID, err2 := uuid.NewRandom() // NOTE: CW expects us to generate an ID, rather than return one
	if err2 != nil {
		return fmt.Errorf("failed to generate UUID: %w", err)
	}
	zero := big.NewInt(0)
	txID := fmt.Sprintf("%s-%s-%s", contract, c.offrampAddress, txUUID.String())
	c.lggr.Infow("Submitting transaction", "tx", txID)
	if err := c.cw.SubmitTransaction(ctx, contract, method, args, txID, c.offrampAddress, &meta, zero); err != nil {
		return fmt.Errorf("failed to submit transaction via chain writer: %w", err)
	}

	return nil
}

func rawReportContext3(digest ocrtypes.ConfigDigest, seqNr uint64) [2][32]byte {
	seqNrBytes := [32]byte{}
	binary.BigEndian.PutUint64(seqNrBytes[24:], seqNr)
	return [2][32]byte{
		digest,
		seqNrBytes,
	}
}

// execCalldataFunc builds the execute call data for Solana.
var execCalldataFunc = func(
	rawReportCtx [2][32]byte,
	report ocr3types.ReportWithInfo[[]byte],
	_, _ [][32]byte,
	_ [32]byte,
	extraDataCodec ccipocr3.ExtraDataCodecBundle,
) (contract string, method string, args any, err error) {
	var info ccipocr3.ExecuteReportInfo
	var extraData ccipocr3.ExtraDataDecoded
	if len(report.Info) != 0 {
		info, err = ccipocr3.DecodeExecuteReportInfo(report.Info)
		if err != nil {
			return "", "", nil, fmt.Errorf("failed to decode execute report info: %w", err)
		}
		if extraDataCodec != nil {
			extraData, err = decodeExecData(info, extraDataCodec)
			if err != nil {
				return "", "", nil, fmt.Errorf("failed to decode extra data: %w", err)
			}
		}
	}

	return consts.ContractNameOffRamp,
		consts.MethodExecute,
		ExecCallArgs{
			ReportContext: rawReportCtx,
			Report:        report.Report,
			Info:          info,
			ExtraData:     extraData,
		}, nil
}

// commitCalldataFunc builds the commit call data for Solana.
// The Solana on-chain contract has two methods, one for the default commit and one for the price-only commit.
var commitCalldataFunc = func(
	rawReportCtx [2][32]byte,
	report ocr3types.ReportWithInfo[[]byte],
	rs, ss [][32]byte,
	vs [32]byte,
	_ ccipocr3.ExtraDataCodecBundle,
) (contract string, method string, args any, err error) {
	var info ccipocr3.CommitReportInfo
	if len(report.Info) != 0 {
		var err error
		info, err = ccipocr3.DecodeCommitReportInfo(report.Info)
		if err != nil {
			return "", "", nil, fmt.Errorf("failed to decode commit report info: %w", err)
		}
	}

	method = consts.MethodCommit
	// Switch to price-only method if no Merkle roots and there are token or gas price updates.
	if len(info.MerkleRoots) == 0 && (len(info.TokenPriceUpdates) > 0 || len(info.GasPriceUpdates) > 0) {
		method = consts.MethodCommitPriceOnly
	}

	return consts.ContractNameOffRamp,
		method,
		commitCallArgs{
			ReportContext: rawReportCtx,
			Report:        report.Report,
			Rs:            rs,
			Ss:            ss,
			RawVs:         vs,
			Info:          info,
		},
		nil
}

// decodeExecData decodes the extra data from an execute report.
func decodeExecData(report ccipocr3.ExecuteReportInfo, codec ccipocr3.ExtraDataCodecBundle) (ccipocr3.ExtraDataDecoded, error) {
	// only one report one message, since this is a stop-gap solution for solana
	if len(report.AbstractReports) != 1 {
		return ccipocr3.ExtraDataDecoded{}, fmt.Errorf("unexpected report length, expected 1, got %d", len(report.AbstractReports))
	}
	if len(report.AbstractReports[0].Messages) != 1 {
		return ccipocr3.ExtraDataDecoded{}, fmt.Errorf("unexpected message length, expected 1, got %d", len(report.AbstractReports[0].Messages))
	}
	message := report.AbstractReports[0].Messages[0]
	extraData := ccipocr3.ExtraDataDecoded{}

	var err error
	extraData.ExtraArgsDecoded, err = codec.DecodeExtraArgs(message.ExtraArgs, report.AbstractReports[0].SourceChainSelector)
	if err != nil {
		return ccipocr3.ExtraDataDecoded{}, fmt.Errorf("failed to decode extra args: %w", err)
	}
	// stopgap solution for missing extra args for Solana. To be replaced in the future.
	destExecDataDecoded := make([]map[string]any, len(message.TokenAmounts))
	for i, tokenAmount := range message.TokenAmounts {
		destExecDataDecoded[i], err = codec.DecodeTokenAmountDestExecData(tokenAmount.DestExecData, report.AbstractReports[0].SourceChainSelector)
		if err != nil {
			return ccipocr3.ExtraDataDecoded{}, fmt.Errorf("failed to decode token amount dest exec data: %w", err)
		}
	}
	extraData.DestExecDataDecoded = destExecDataDecoded

	return extraData, nil
}
