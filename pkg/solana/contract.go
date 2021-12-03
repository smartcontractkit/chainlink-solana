package solana

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/pkg/errors"
	"go.uber.org/atomic"
)

var (
	configVersion         uint8 = 1
	errAlreadyTriggered         = errors.New("Observe (job run) has already been triggered")
	errFetchCtxCancelled        = errors.New("fetch context cancelled")
	errCursorLength             = errors.New("incorrect cursor length")
	errTransmissionLength       = errors.New("incorrect transmission length")
)

type ContractTracker struct {
	// on-chain program + 2x state accounts (state + transmissions)
	ProgramID       solana.PublicKey
	StateID         solana.PublicKey
	TransmissionsID solana.PublicKey

	// private key for the transmission signing
	Transmitter solana.PrivateKey

	client                *Client
	state                 State
	answer                Answer
	lockState             *atomic.Bool
	lockStateChan         chan struct{}
	lockTransmissions     *atomic.Bool
	lockTransmissionsChan chan struct{}
}

func NewTracker(address string, jobID string, client *Client) (*ContractTracker, error) {
	// <program account>-<state account>-<transmission account>
	// TODO: validate that this format is followed
	accounts := strings.Split(address, "-")

	pubKeys := []solana.PublicKey{}
	for _, a := range accounts {
		pubKey, err := solana.PublicKeyFromBase58(a)
		if err != nil {
			return &ContractTracker{}, err
		}
		pubKeys = append(pubKeys, pubKey)
	}

	// TODO: @Blaz/@Ryan the solana-go requires a private key (?)
	transmitter, err := solana.NewRandomPrivateKey()
	if err != nil {
		return nil, err
	}

	return &ContractTracker{
		ProgramID:         pubKeys[0],
		StateID:           pubKeys[1],
		TransmissionsID:   pubKeys[2],
		Transmitter:       transmitter,
		client:            client,
		lockState:         atomic.NewBool(false), // initialize to unlocked
		lockTransmissions: atomic.NewBool(false), // initialize to unlocked
	}, nil
}

func (c *ContractTracker) Start() {}

func (c *ContractTracker) Close() error {
	return nil
}

// fetch wraps fetchState or fetchTransmissions with a lock
// allows for `fetch` function to be called multiple times, but will not spam the underlying "get data" requests
// example:
// - without any locks, etc: fetchState could be called by multiple components in the config tracker, median contract, etc
//   this would slow the system down since each request is a new request to the endpoint
// - with `fetchWrap` + locks: fetchState could be called by multiple components without sending a new request to the endpoint if there is already one running
//   first request would lock
//   second request would hit an error because of the lock, and wait for a signal that the request is fulfilled (channel closure)
//   first + second request complets and allows the calling functions to then read from state for the required parameters
func fetchWrap(ctx context.Context, fetch func(context.Context) error, done *chan struct{}) error {
	log.Print("fetching account data")
	err := fetch(ctx)

	switch err {
	case nil: // occurs when fetch successfully completed
		// do nothing
	case errAlreadyTriggered: // occurs when fetch function has already been called
		log.Print("fetch already triggered, waiting for completion")
		select {
		case <-*done:
			// continue
		case <-ctx.Done():
			return errFetchCtxCancelled
		}
	default: // return if unrecognized error
		return err
	}

	log.Print("fetch complete")
	return nil
}

func (c *ContractTracker) fetchState(ctx context.Context) error {
	// lock
	if !c.lockState.CAS(false, true) {
		return errAlreadyTriggered
	}
	defer c.lockState.Store(false)

	// create channel to announce done
	c.lockStateChan = make(chan struct{})
	defer close(c.lockStateChan)

	// fetch + decode + store raw state
	log.Printf("fetch state account: %s", c.StateID.String())

	if err := c.client.rpc.GetAccountDataInto(ctx, c.StateID, &c.state); err != nil {
		return err
	}

	// validation for config version
	if configVersion != c.state.Config.Version {
		return fmt.Errorf("decoded config version (%d) does not match expected config version (%d)", c.state.Config.Version, configVersion)
	}
	return nil
}

func (c *ContractTracker) fetchTransmissions(ctx context.Context) error {
	// lock
	if !c.lockTransmissions.CAS(false, true) {
		return errAlreadyTriggered
	}
	defer c.lockTransmissions.Store(false)

	// create channel to announce done
	c.lockTransmissionsChan = make(chan struct{})
	defer close(c.lockTransmissionsChan)

	log.Printf("fetch transmissions account: %s", c.TransmissionsID.String())
	a, err := fetchTransmissionsState(ctx, c.client.rpc, c.TransmissionsID)
	c.answer = a
	return err
}

func fetchTransmissionsState(ctx context.Context, client *rpc.Client, account solana.PublicKey) (Answer, error) {
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
		return Answer{}, errors.Wrap(err, "error on rpc.GetAccountInfo [cursor]")
	}

	// parse little endian cursor value
	c := res.Value.Data.GetBinary()
	if len(c) != int(cursorLen) { // validate length
		return Answer{}, errCursorLength
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
		return Answer{}, errors.Wrap(err, "error on rpc.GetAccountInfo [transmission]")
	}

	t := res.Value.Data.GetBinary()
	if len(t) != int(transmissionLen) { // validate length
		return Answer{}, errTransmissionLength
	}

	// reverse slice to change from little endian to big endian
	for i, j := 0, len(t)-1; i < j; i, j = i+1, j-1 {
		t[i], t[j] = t[j], t[i]
	}

	return Answer{
		Answer:    big.NewInt(0).SetBytes(t[4:]),
		Timestamp: binary.BigEndian.Uint32(t[:4]),
	}, nil
}
