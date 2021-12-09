package solana

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/big"

	"golang.org/x/sync/singleflight"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/pkg/errors"
)

var (
	configVersion uint8 = 1

	// error declarations
	errCursorLength       = errors.New("incorrect cursor length")
	errTransmissionLength = errors.New("incorrect transmission length")
)

type ContractTracker struct {
	// on-chain program + 2x state accounts (state + transmissions)
	ProgramID          solana.PublicKey
	StateID            solana.PublicKey
	TransmissionsID    solana.PublicKey
	ValidatorProgramID solana.PublicKey

	// private key for the transmission signing
	Transmitter TransmissionSigner

	// tracked contract state
	state  State
	answer Answer

	// dependencies
	client *Client
	lggr   Logger

	// provides a duplicate function call suppression mechanism
	requestGroup *singleflight.Group
}

func NewTracker(spec OCR2Spec, client *Client, transmitter TransmissionSigner, lggr Logger) ContractTracker {
	return ContractTracker{
		ProgramID:          spec.ProgramID,
		StateID:            spec.StateID,
		ValidatorProgramID: spec.ValidatorProgramID,
		TransmissionsID:    spec.TransmissionsID,
		Transmitter:        transmitter,
		client:             client,
		lggr:               lggr,
		requestGroup:       &singleflight.Group{},
	}
}

// fetch + decode + store raw state
func (c *ContractTracker) fetchState(ctx context.Context) error {
	c.lggr.Debugf("fetch state for account: %s", c.StateID.String())

	// make single flight request
	v, err, shared := c.requestGroup.Do("state", func() (interface{}, error) {
		return getState(ctx, c.client.rpc, c.StateID)
	})

	if err != nil {
		return err
	}

	c.lggr.Debugf("state fetched for account: %s, shared: %t, result: %v", c.StateID, shared, v)

	c.state = v.(State)
	return nil
}

func (c *ContractTracker) fetchLatestTransmission(ctx context.Context) error {
	c.lggr.Debugf("fetch latest transmission for account: %s", c.TransmissionsID)

	// make single flight request
	v, err, shared := c.requestGroup.Do("transmissions.latest", func() (interface{}, error) {
		answer, _, err := GetLatestTransmission(ctx, c.client.rpc, c.TransmissionsID)
		return answer, err
	})

	if err != nil {
		return err
	}

	c.lggr.Debugf("latest transmission fetched for account: %s, shared: %t, result: %v", c.TransmissionsID, shared, v)

	c.answer = v.(Answer)
	return nil
}

func getState(ctx context.Context, client *rpc.Client, account solana.PublicKey) (State, error) {
	var state State
	if err := client.GetAccountDataInto(ctx, account, &state); err != nil {
		return state, err
	}

	// validation for config version
	if configVersion != state.Config.Version {
		return State{}, fmt.Errorf("decoded config version (%d) does not match expected config version (%d)", state.Config.Version, configVersion)
	}

	return state, nil
}

func GetLatestTransmission(ctx context.Context, client *rpc.Client, account solana.PublicKey) (Answer, uint64, error) {
	cursorOffset := CursorOffset
	cursorLen := CursorLen
	transmissionLen := TransmissionLen

	// query for cursor
	res, err := client.GetAccountInfoWithOpts(ctx, account, &rpc.GetAccountInfoOpts{
		DataSlice: &rpc.DataSlice{
			Offset: &cursorOffset,
			Length: &cursorLen,
		},
	})
	if err != nil {
		return Answer{}, 0, errors.Wrap(err, "error on rpc.GetAccountInfo [cursor]")
	}

	// parse little endian cursor value
	c := res.Value.Data.GetBinary()
	if len(c) != int(cursorLen) { // validate length
		return Answer{}, 0, errCursorLength
	}
	cursor := binary.LittleEndian.Uint32(c)
	if cursor == 0 { // handle array wrap
		cursor = TransmissionsSize
	}
	cursor-- // cursor indicates index for new answer, latest answer is in previous index

	// fetch transmission
	var transmissionOffset uint64 = CursorOffset + CursorLen + (uint64(cursor) * transmissionLen)
	res, err = client.GetAccountInfoWithOpts(ctx, account, &rpc.GetAccountInfoOpts{
		DataSlice: &rpc.DataSlice{
			Offset: &transmissionOffset,
			Length: &transmissionLen,
		},
	})
	if err != nil {
		return Answer{}, 0, errors.Wrap(err, "error on rpc.GetAccountInfo [transmission]")
	}

	t := res.Value.Data.GetBinary()
	if len(t) != int(transmissionLen) { // validate length
		return Answer{}, 0, errTransmissionLength
	}

	// reverse slice to change from little endian to big endian
	for i, j := 0, len(t)-1; i < j; i, j = i+1, j-1 {
		t[i], t[j] = t[j], t[i]
	}

	return Answer{
		Data:      big.NewInt(0).SetBytes(t[4:]),
		Timestamp: binary.BigEndian.Uint32(t[:4]),
	}, res.RPCContext.Context.Slot, nil
}
