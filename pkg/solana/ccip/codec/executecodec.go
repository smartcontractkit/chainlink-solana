package codec

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"

	agbinary "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-common/pkg/types/ccipocr3"

	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/latest/ccip_offramp"
)

// ExecutePluginCodecV1 is a codec for encoding and decoding execute plugin reports.
// Compatible with:
// - "OffRamp 1.6.0-dev"
type ExecutePluginCodecV1 struct {
	extraDataCodec ccipocr3.ExtraDataCodecBundle
}

// ExtraData is a struct for holding the decoded extra args data used for encoding the execute report.
type ExtraData struct {
	ExtraArgs     ccip_offramp.Any2SVMRampExtraArgs
	Accounts      []solana.PublicKey
	TokenReceiver solana.PublicKey
}

func NewExecutePluginCodecV1(extraDataCodec ccipocr3.ExtraDataCodecBundle) *ExecutePluginCodecV1 {
	return &ExecutePluginCodecV1{
		extraDataCodec: extraDataCodec,
	}
}

func (e *ExecutePluginCodecV1) Encode(ctx context.Context, report ccipocr3.ExecutePluginReport) ([]byte, error) {
	if len(report.ChainReports) == 0 {
		// OCR3 runs in a constant loop and will produce empty reports, so we need to handle this case
		// return an empty report, CCIP will discard it on ShouldAcceptAttestedReport/ShouldTransmitAcceptedReport
		// via validateReport before attempting to decode
		return nil, nil
	}

	if len(report.ChainReports) != 1 {
		return nil, fmt.Errorf("unexpected chain report length: %d", len(report.ChainReports))
	}

	chainReport := report.ChainReports[0]
	if len(chainReport.Messages) > 1 {
		return nil, fmt.Errorf("unexpected report message length: %d", len(chainReport.Messages))
	}

	var message ccip_offramp.Any2SVMRampMessage
	var offChainTokenData [][]byte
	if len(chainReport.Messages) > 0 {
		// currently only allow executing one message at a time
		msg := chainReport.Messages[0]
		tokenAmounts := make([]ccip_offramp.Any2SVMTokenTransfer, 0, len(msg.TokenAmounts))
		for _, tokenAmount := range msg.TokenAmounts {
			if tokenAmount.Amount.IsEmpty() {
				return nil, fmt.Errorf("empty amount for token: %s", tokenAmount.DestTokenAddress)
			}

			if tokenAmount.Amount.Int.Sign() < 0 {
				return nil, fmt.Errorf("negative amount for token: %s", tokenAmount.DestTokenAddress)
			}

			if len(tokenAmount.DestTokenAddress) != solana.PublicKeyLength {
				return nil, fmt.Errorf("invalid destTokenAddress address: %v", tokenAmount.DestTokenAddress)
			}

			destExecDataDecodedMap, err := e.extraDataCodec.DecodeTokenAmountDestExecData(tokenAmount.DestExecData, chainReport.SourceChainSelector)
			if err != nil {
				return nil, fmt.Errorf("failed to decode dest exec data: %w", err)
			}

			destGasAmount, err := ExtractDestGasAmountFromMap(destExecDataDecodedMap)
			if err != nil {
				return nil, err
			}

			tokenAmounts = append(tokenAmounts, ccip_offramp.Any2SVMTokenTransfer{
				SourcePoolAddress: tokenAmount.SourcePoolAddress,
				DestTokenAddress:  solana.PublicKeyFromBytes(tokenAmount.DestTokenAddress),
				ExtraData:         tokenAmount.ExtraData,
				Amount:            ccip_offramp.CrossChainAmount{LeBytes: [32]uint8(encodeBigIntToFixedLengthLE(tokenAmount.Amount.Int, 32))},
				DestGasAmount:     destGasAmount,
			})
		}

		extraDataDecodedMap, err := e.extraDataCodec.DecodeExtraArgs(msg.ExtraArgs, chainReport.SourceChainSelector)
		if err != nil {
			return nil, fmt.Errorf("failed to decode extra args: %w", err)
		}

		ed, err := ParseExtraDataMap(extraDataDecodedMap)
		if err != nil {
			return nil, fmt.Errorf("invalid extra args map: %w", err)
		}

		if len(msg.Receiver) != solana.PublicKeyLength {
			return nil, fmt.Errorf("invalid receiver address: %v", msg.Receiver)
		}

		message = ccip_offramp.Any2SVMRampMessage{
			Header: ccip_offramp.RampMessageHeader{
				MessageId:           msg.Header.MessageID,
				SourceChainSelector: uint64(msg.Header.SourceChainSelector),
				DestChainSelector:   uint64(msg.Header.DestChainSelector),
				SequenceNumber:      uint64(msg.Header.SequenceNumber),
				Nonce:               msg.Header.Nonce,
			},
			Sender:        msg.Sender,
			Data:          msg.Data,
			TokenReceiver: ed.TokenReceiver,
			TokenAmounts:  tokenAmounts,
			ExtraArgs:     ed.ExtraArgs,
		}

		// should only have an offchain token data if there are tokens as part of the message
		if len(chainReport.OffchainTokenData) > 0 {
			offChainTokenData = chainReport.OffchainTokenData[0]
		}
	}

	solanaProofs := make([][32]byte, 0, len(chainReport.Proofs))
	for _, proof := range chainReport.Proofs {
		solanaProofs = append(solanaProofs, proof)
	}

	solanaReport := ccip_offramp.ExecutionReportSingleChain{
		SourceChainSelector: uint64(chainReport.SourceChainSelector),
		Message:             message,
		OffchainTokenData:   offChainTokenData,
		Proofs:              solanaProofs,
	}

	var buf bytes.Buffer
	encoder := agbinary.NewBorshEncoder(&buf)
	err := solanaReport.MarshalWithEncoder(encoder)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (e *ExecutePluginCodecV1) Decode(ctx context.Context, encodedReport []byte) (ccipocr3.ExecutePluginReport, error) {
	decoder := agbinary.NewBorshDecoder(encodedReport)
	executeReport := ccip_offramp.ExecutionReportSingleChain{}
	err := executeReport.UnmarshalWithDecoder(decoder)
	if err != nil {
		return ccipocr3.ExecutePluginReport{}, fmt.Errorf("unpack encoded report: %w", err)
	}

	tokenAmounts := make([]ccipocr3.RampTokenAmount, 0, len(executeReport.Message.TokenAmounts))
	for _, tokenAmount := range executeReport.Message.TokenAmounts {
		destData := make([]byte, 4)
		binary.LittleEndian.PutUint32(destData, tokenAmount.DestGasAmount)

		tokenAmounts = append(tokenAmounts, ccipocr3.RampTokenAmount{
			SourcePoolAddress: tokenAmount.SourcePoolAddress,
			DestTokenAddress:  tokenAmount.DestTokenAddress.Bytes(),
			ExtraData:         tokenAmount.ExtraData,
			Amount:            decodeLEToBigInt(tokenAmount.Amount.LeBytes[:]),
			DestExecData:      destData,
		})
	}

	var buf bytes.Buffer
	encoder := agbinary.NewBorshEncoder(&buf)
	err = executeReport.Message.ExtraArgs.MarshalWithEncoder(encoder)
	if err != nil {
		return ccipocr3.ExecutePluginReport{}, fmt.Errorf("unpack encoded report: %w", err)
	}

	messages := []ccipocr3.Message{
		{
			Header: ccipocr3.RampMessageHeader{
				MessageID:           executeReport.Message.Header.MessageId,
				SourceChainSelector: ccipocr3.ChainSelector(executeReport.Message.Header.SourceChainSelector),
				DestChainSelector:   ccipocr3.ChainSelector(executeReport.Message.Header.DestChainSelector),
				SequenceNumber:      ccipocr3.SeqNum(executeReport.Message.Header.SequenceNumber),
				Nonce:               executeReport.Message.Header.Nonce,
				MsgHash:             ccipocr3.Bytes32{},        // todo: info not available, but not required atm
				OnRamp:              ccipocr3.UnknownAddress{}, // todo: info not available, but not required atm
			},
			Sender:         executeReport.Message.Sender,
			Data:           executeReport.Message.Data,
			Receiver:       executeReport.Message.TokenReceiver.Bytes(),
			ExtraArgs:      buf.Bytes(),
			FeeToken:       ccipocr3.UnknownAddress{}, // <-- todo: info not available, but not required atm
			FeeTokenAmount: ccipocr3.BigInt{},         // <-- todo: info not available, but not required atm
			TokenAmounts:   tokenAmounts,
		},
	}

	offchainTokenData := make([][][]byte, 0, 1)
	if executeReport.OffchainTokenData != nil {
		offchainTokenData = append(offchainTokenData, executeReport.OffchainTokenData)
	}

	proofs := make([]ccipocr3.Bytes32, 0, len(executeReport.Proofs))
	for _, proof := range executeReport.Proofs {
		proofs = append(proofs, proof)
	}

	chainReport := ccipocr3.ExecutePluginReportSingleChain{
		SourceChainSelector: ccipocr3.ChainSelector(executeReport.SourceChainSelector),
		Messages:            messages,
		OffchainTokenData:   offchainTokenData,
		Proofs:              proofs,
	}

	report := ccipocr3.ExecutePluginReport{
		ChainReports: []ccipocr3.ExecutePluginReportSingleChain{chainReport},
	}

	return report, nil
}

// ParseExtraDataMap parses the decoded extra args map into the extraData struct used for encoding the execute report.
func ParseExtraDataMap(input map[string]any) (ExtraData, error) {
	// Parse input map into SolanaExtraArgs
	var out ExtraData
	var extraArgs ccip_offramp.Any2SVMRampExtraArgs
	var accounts []solana.PublicKey
	var tokenReceiver solana.PublicKey

	// Iterate through the expected fields in the struct.
	// The field names should match SVMExtraArgsV1 from the EVM Client library:
	// https://github.com/smartcontractkit/chainlink/blob/33c0bda696b0ed97f587a46eacd5c65bed9fb2c1/contracts/src/v0.8/ccip/libraries/Client.sol#L57
	//
	// Note: when the source chain ExtraDataCodec runs in a LOOP plugin (e.g. TON relay),
	// the map[string]any values go through gRPC protobuf serialization which changes types:
	//   uint32 -> int64, [32]byte -> []byte, [][32]byte -> []interface{}
	// Each case below handles both the native type and the LOOP-converted type.
	for fieldName, fieldValue := range input {
		lowercase := strings.ToLower(fieldName)
		switch lowercase {
		case "computeunits":
			switch v := fieldValue.(type) {
			case uint32:
				extraArgs.ComputeUnits = v
			case int64: // LOOP gRPC converts uint32 -> int64
				if v < 0 || v > math.MaxUint32 {
					return out, fmt.Errorf("ComputeUnits out of uint32 range: %d", v)
				}
				extraArgs.ComputeUnits = uint32(v)
			default:
				return out, fmt.Errorf("invalid type for ComputeUnits, expected uint32, got %T", fieldValue)
			}
		case "accountiswritablebitmap":
			switch v := fieldValue.(type) {
			case uint64:
				extraArgs.IsWritableBitmap = v
			case int64: // LOOP gRPC may convert uint64 -> int64
				extraArgs.IsWritableBitmap = uint64(v) //nolint:gosec // bitmap, interpret bits as-is
			default:
				return out, fmt.Errorf("invalid type for IsWritableBitmap, expected uint64, got %T", fieldValue)
			}
		case "Accounts":
			switch v := fieldValue.(type) {
			case [][32]byte:
				a := make([]solana.PublicKey, len(v))
				for i, val := range v {
					a[i] = solana.PublicKeyFromBytes(val[:])
				}
				accounts = a
			case []interface{}: // LOOP gRPC converts [][32]byte -> []interface{}
				a := make([]solana.PublicKey, len(v))
				for i, elem := range v {
					bs, ok := elem.([]byte)
					if !ok {
						return out, fmt.Errorf("invalid type for Accounts[%d], expected []byte, got %T", i, elem)
					}
					if len(bs) != 32 {
						return out, fmt.Errorf("invalid length for Accounts[%d]: expected 32, got %d", i, len(bs))
					}
					a[i] = solana.PublicKeyFromBytes(bs)
				}
				accounts = a
			case [][]byte: // alternative LOOP representation
				a := make([]solana.PublicKey, len(v))
				for i, bs := range v {
					if len(bs) != 32 {
						return out, fmt.Errorf("invalid length for Accounts[%d]: expected 32, got %d", i, len(bs))
					}
					a[i] = solana.PublicKeyFromBytes(bs)
				}
				accounts = a
			default:
				return out, fmt.Errorf("invalid type for Accounts, expected [][32]byte, got %T", fieldValue)
			}
		case "tokenreceiver":
			switch v := fieldValue.(type) {
			case [32]byte:
				tokenReceiver = solana.PublicKeyFromBytes(v[:])
			case []byte: // LOOP gRPC converts [32]byte -> []byte
				if len(v) != 32 {
					return out, fmt.Errorf("invalid length for TokenReceiver: expected 32, got %d", len(v))
				}
				tokenReceiver = solana.PublicKeyFromBytes(v)
			default:
				return out, fmt.Errorf("invalid type for TokenReceiver, expected [32]byte, got %T", fieldValue)
			}
		default:
			// no error here, unneeded keys can be skipped without return errors
		}
	}

	out.ExtraArgs = extraArgs
	out.Accounts = accounts
	out.TokenReceiver = tokenReceiver
	return out, nil
}

// ExtractDestGasAmountFromMap searches for the dest gas amount in the decoded DestExecData map and returns it as a uint32.
func ExtractDestGasAmountFromMap(input map[string]any) (uint32, error) {
	// Search for the gas fields
	for fieldName, fieldValue := range input {
		lowercase := strings.ToLower(fieldName)
		switch lowercase {
		case "destgasamount":
			switch v := fieldValue.(type) {
			case uint32:
				return v, nil
			case int64: // LOOP converts expected uint32 to int64
				if v > math.MaxUint32 {
					return 0, fmt.Errorf("destGasAmount exceeds uint32 max, got %d", v)
				}
				return uint32(v), nil //nolint:gosec // G115: validated to be within uint32 max above
			default:
				return 0, fmt.Errorf("invalid type for destgasamount, expected uint32 or int64, got %T", v)
			}
		default:
		}
	}

	return 0, errors.New("invalid token message, dest gas amount not found in the DestExecDataDecoded map")
}

func encodeBigIntToFixedLengthLE(bi *big.Int, length int) []byte {
	// Create a fixed-length byte array
	paddedBytes := make([]byte, length)

	// Use FillBytes to fill the array with big-endian data, zero-padded
	bi.FillBytes(paddedBytes)

	// Reverse the array for little-endian encoding
	for i, j := 0, len(paddedBytes)-1; i < j; i, j = i+1, j-1 {
		paddedBytes[i], paddedBytes[j] = paddedBytes[j], paddedBytes[i]
	}

	return paddedBytes
}

func decodeLEToBigInt(data []byte) ccipocr3.BigInt {
	// Avoid modifying original data
	buf := make([]byte, len(data))
	copy(buf, data)

	// Reverse the byte array to convert it from little-endian to big-endian
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}

	// Use big.Int.SetBytes to construct the big.Int
	bi := new(big.Int).SetBytes(buf)
	if bi.Cmp(big.NewInt(0)) == 0 {
		return ccipocr3.NewBigInt(big.NewInt(0))
	}

	return ccipocr3.NewBigInt(bi)
}

// Ensure ExecutePluginCodec implements the ExecutePluginCodec interface
var _ ccipocr3.ExecutePluginCodec = (*ExecutePluginCodecV1)(nil)
