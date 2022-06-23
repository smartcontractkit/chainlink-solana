package monitoring

import (
	"fmt"
	"time"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"github.com/smartcontractkit/chainlink-relay/pkg/monitoring/pb"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/event"
	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
	"google.golang.org/protobuf/proto"
)

type Decoder func(interface{}, SolanaConfig, SolanaFeedConfig) (interface{}, error)

type StateAccount struct {
	Slot       uint64
	Lamports   uint64
	Owner      solana.PublicKey
	Executable bool
	RentEpoch  uint64

	State                 pkgSolana.State
	OffchainConfig        pb.OffchainConfigProto
	NumericalMedianConfig pb.NumericalMedianConfigProto
}

func StateResultDecoder(raw interface{}, _ SolanaConfig, _ SolanaFeedConfig) (interface{}, error) {
	result, isResult := raw.(*ws.AccountResult)
	if !isResult {
		return nil, fmt.Errorf("expected input of type *ws.AccountResult, instead got '%T'", raw)
	}
	state := pkgSolana.State{}
	if err := bin.NewBinDecoder(result.Value.Data.GetBinary()).Decode(&state); err != nil {
		return nil, fmt.Errorf("failed to decode state account: %w", err)
	}
	rawOffchainConfig, err := state.OffchainConfig.Data()
	if err != nil {
		return nil, fmt.Errorf("incorrect offchain config data: %w", err)
	}
	offchainConfig := pb.OffchainConfigProto{}
	if err := proto.Unmarshal(rawOffchainConfig, &offchainConfig); err != nil {
		return nil, fmt.Errorf("failed to decode offchain config: %w", err)
	}
	numericalMedianConfig := pb.NumericalMedianConfigProto{}
	if err := proto.Unmarshal(offchainConfig.ReportingPluginConfig, &numericalMedianConfig); err != nil {
		return nil, fmt.Errorf("failed to decode reporting plugin config: %w", err)
	}
	return StateAccount{
		result.Context.Slot,
		result.Value.Lamports,
		result.Value.Owner,
		result.Value.Executable,
		result.Value.RentEpoch,
		state,
		offchainConfig,
		numericalMedianConfig,
	}, nil
}

type TransmissionsAccount struct {
	Slot       uint64
	Lamports   uint64
	Owner      solana.PublicKey
	Executable bool
	RentEpoch  uint64

	Header       pkgSolana.TransmissionsHeader
	Transmission pkgSolana.Transmission
}

func TransmissionResultDecoder(raw interface{}, _ SolanaConfig, _ SolanaFeedConfig) (interface{}, error) {
	result, isResult := raw.(*ws.AccountResult)
	if !isResult {
		return nil, fmt.Errorf("expected input of type *ws.AccountResult, instead got '%T'", raw)
	}
	data := result.Value.Data.GetBinary()
	// Parse header.
	rawHeader := data[pkgSolana.AccountDiscriminatorLen : pkgSolana.AccountDiscriminatorLen+pkgSolana.TransmissionsHeaderLen]
	var header pkgSolana.TransmissionsHeader
	if err := bin.NewBinDecoder(rawHeader).Decode(&header); err != nil {
		return nil, fmt.Errorf("failed to decode transmission account header: %w", err)
	}
	// Parse transmission.
	cursor := header.LiveCursor
	if cursor == 0 { // handle array wrap
		cursor = header.LiveLength
	}
	cursor-- // cursor indicates index for new answer, latest answer is in previous index
	transmissionOffset := pkgSolana.AccountDiscriminatorLen + pkgSolana.TransmissionsHeaderMaxSize + (uint64(cursor) * pkgSolana.TransmissionLen)
	transmissionRaw := data[transmissionOffset : transmissionOffset+pkgSolana.TransmissionLen]
	var transmission pkgSolana.Transmission
	if err := bin.NewBinDecoder(transmissionRaw).Decode(&transmission); err != nil {
		return nil, fmt.Errorf("failed to decode latest transmission: %w", err)
	}
	return TransmissionsAccount{
		result.Context.Slot,
		result.Value.Lamports,
		result.Value.Owner,
		result.Value.Executable,
		result.Value.RentEpoch,
		header,
		transmission,
	}, nil
}

type Logs struct {
	Slot      uint64
	Signature solana.Signature
	Err       string

	Events []interface{}
}

func LogResultDecode(raw interface{}, _ SolanaConfig, config SolanaFeedConfig) (interface{}, error) {
	result, isResult := raw.(*ws.LogResult)
	if !isResult {
		return nil, fmt.Errorf("expected input of type *ws.BlockResult, instead got '%T'", raw)
	}
	if result.Value.Err != nil {
		return nil, errNoResults
	}

	encodedEvents := event.ExtractEvents(result.Value.Logs, config.ContractAddressBase58)

	events, err := event.DecodeMultiple(encodedEvents)
	if err != nil {
		return nil, fmt.Errorf("failed to decode event: %w", err)
	}
	logsErr := ""
	if err, ok := result.Value.Err.(error); ok {
		logsErr = err.Error()
	}
	return Logs{
		result.Context.Slot,
		result.Value.Signature,
		logsErr,
		events,
	}, nil
}

type Block struct {
	Slot              uint64
	Err               string
	Blockhash         solana.Hash
	PreviousBlockhash solana.Hash
	ParentSlot        uint64
	BlockTime         time.Time
	BlockHeight       uint64

	Transactions []rpc.TransactionWithMeta
	Rewards      []rpc.BlockReward
}

func BlockResultDecode(raw interface{}, _ SolanaConfig, config SolanaFeedConfig) (interface{}, error) {
	result, isResult := raw.(*ws.BlockResult)
	if !isResult {
		return nil, fmt.Errorf("expected input of type *ws.LogResult, instead got '%T'", raw)
	}
	block := Block{
		Slot: result.Context.Slot,
		Err:  fmt.Sprintf("%s", result.Value.Err),
	}
	if result.Value.Err != nil {
		return block, nil
	}
	block.Blockhash = result.Value.Block.Blockhash
	block.PreviousBlockhash = result.Value.Block.PreviousBlockhash
	block.ParentSlot = result.Value.Block.ParentSlot
	if result.Value.Block.BlockTime != nil {
		block.BlockTime = result.Value.Block.BlockTime.Time()
	}
	if result.Value.Block.BlockHeight != nil {
		block.BlockHeight = *result.Value.Block.BlockHeight
	}
	block.Transactions = result.Value.Block.Transactions
	block.Rewards = result.Value.Block.Rewards
	return block, nil
}
