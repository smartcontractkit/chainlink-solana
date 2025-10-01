package codec

import (
	"context"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/ccip"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/types/ccipocr3"
)

// MessageHasherV1 implements the MessageHasher interface.
// Compatible with:
// - "OnRamp 1.6.0-dev"
type MessageHasherV1 struct {
	lggr           logger.Logger
	extraDataCodec ccipocr3.ExtraDataCodecBundle
}

func NewMessageHasherV1(lggr logger.Logger, extraDataCodec ccipocr3.ExtraDataCodecBundle) *MessageHasherV1 {
	return &MessageHasherV1{
		lggr:           lggr,
		extraDataCodec: extraDataCodec,
	}
}

// Hash implements the MessageHasher interface.
func (h *MessageHasherV1) Hash(_ context.Context, msg ccipocr3.Message) (ccipocr3.Bytes32, error) {
	h.lggr.Debugw("hashing message", "msg", msg)

	anyToSolanaMessage := ccip_offramp.Any2SVMRampMessage{}
	anyToSolanaMessage.Header = ccip_offramp.RampMessageHeader{
		SourceChainSelector: uint64(msg.Header.SourceChainSelector),
		DestChainSelector:   uint64(msg.Header.DestChainSelector),
		SequenceNumber:      uint64(msg.Header.SequenceNumber),
		MessageId:           msg.Header.MessageID,
		Nonce:               msg.Header.Nonce,
	}
	if solana.PublicKeyLength != len(msg.Receiver) {
		return [32]byte{}, fmt.Errorf("invalid receiver length: %d", len(msg.Receiver))
	}

	anyToSolanaMessage.Sender = msg.Sender
	anyToSolanaMessage.Data = msg.Data
	for _, ta := range msg.TokenAmounts {
		destExecDataDecodedMap, err := h.extraDataCodec.DecodeTokenAmountDestExecData(ta.DestExecData, msg.Header.SourceChainSelector)
		if err != nil {
			return [32]byte{}, fmt.Errorf("failed to decode dest exec data: %w", err)
		}

		destGasAmount, err := extractDestGasAmountFromMap(destExecDataDecodedMap)
		if err != nil {
			return [32]byte{}, err
		}

		if solana.PublicKeyLength != len(ta.DestTokenAddress) {
			return [32]byte{}, fmt.Errorf("invalid DestTokenAddress length: %d", len(ta.DestTokenAddress))
		}
		anyToSolanaMessage.TokenAmounts = append(anyToSolanaMessage.TokenAmounts, ccip_offramp.Any2SVMTokenTransfer{
			SourcePoolAddress: ta.SourcePoolAddress,
			DestTokenAddress:  solana.PublicKeyFromBytes(ta.DestTokenAddress),
			ExtraData:         ta.ExtraData,
			DestGasAmount:     destGasAmount,
			Amount:            ccip_offramp.CrossChainAmount{LeBytes: [32]uint8(encodeBigIntToFixedLengthLE(ta.Amount.Int, 32))},
		})
	}

	extraDataDecodedMap, err := h.extraDataCodec.DecodeExtraArgs(msg.ExtraArgs, msg.Header.SourceChainSelector)
	if err != nil {
		return [32]byte{}, fmt.Errorf("failed to decode extra args: %w", err)
	}

	ed, err := parseExtraDataMap(extraDataDecodedMap)
	if err != nil {
		return [32]byte{}, fmt.Errorf("failed to decode ExtraArgs: %w", err)
	}

	anyToSolanaMessage.TokenReceiver = ed.tokenReceiver
	anyToSolanaMessage.ExtraArgs = ed.extraArgs
	accounts := ed.accounts
	// if logical receiver is empty, don't prepend it to the accounts list
	if !msg.Receiver.IsZeroOrEmpty() {
		accounts = append([]solana.PublicKey{solana.PublicKeyFromBytes(msg.Receiver)}, accounts...)
	}

	hash, err := ccip.HashAnyToSVMMessage(anyToSolanaMessage, msg.Header.OnRamp, accounts)
	return [32]byte(hash), err
}

func SerializeExtraArgs(tag []byte, data any) ([]byte, error) {
	return ccip.SerializeExtraArgs(data, strings.TrimPrefix(hexutil.Encode(tag), "0x"))
}

// Interface compliance check
var _ ccipocr3.MessageHasher = (*MessageHasherV1)(nil)
