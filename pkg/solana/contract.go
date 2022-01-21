package solana

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/pkg/errors"
	"github.com/smartcontractkit/chainlink/core/utils"
)

var (
	configVersion       uint8 = 1
	defaultStaleTimeout       = 1 * time.Minute

	// error declarations
	errCursorLength       = errors.New("incorrect cursor length")
	errTransmissionLength = errors.New("incorrect transmission length")
)

type ContractTracker struct {
	// on-chain program + 2x state accounts (state + transmissions)
	ProgramID       solana.PublicKey
	StateID         solana.PublicKey
	TransmissionsID solana.PublicKey
	StoreProgramID  solana.PublicKey

	// private key for the transmission signing
	Transmitter TransmissionSigner

	// tracked contract state
	state  State
	store  solana.PublicKey
	answer Answer

	// read/write mutexes
	stateLock *sync.RWMutex
	ansLock   *sync.RWMutex

	// stale state parameters
	stateTime    time.Time
	ansTime      time.Time
	staleTimeout time.Duration

	// dependencies
	client *Client
	lggr   Logger

	// provides a duplicate function call suppression mechanism
	requestGroup *singleflight.Group

	// polling
	done chan struct{}
	utils.StartStopOnce
}

func NewTracker(spec OCR2Spec, client *Client, transmitter TransmissionSigner, lggr Logger) ContractTracker {
	// parse poll interval, if errors: use 1 second default
	staleTimeout, err := time.ParseDuration(spec.StaleTimeout)
	if err != nil {
		lggr.Warnf("could not parse stale timeout interval using default 1m")
		staleTimeout = defaultStaleTimeout
	}

	return ContractTracker{
		ProgramID:       spec.ProgramID,
		StateID:         spec.StateID,
		StoreProgramID:  spec.StoreProgramID,
		TransmissionsID: spec.TransmissionsID,
		Transmitter:     transmitter,
		client:          client,
		lggr:            lggr,
		requestGroup:    &singleflight.Group{},
		stateLock:       &sync.RWMutex{},
		ansLock:         &sync.RWMutex{},
		stateTime:       time.Now(),
		ansTime:         time.Now(),
		staleTimeout:    staleTimeout,
	}
}

// Start polling
func (c *ContractTracker) Start() error {
	return c.StartOnce("pollState", func() error {
		c.done = make(chan struct{})
		go c.PollState()
		return nil
	})
}

// PollState contains the state and transmissions polling implementation
func (c *ContractTracker) PollState() {
	c.lggr.Debugf("Starting state polling for state: %s, transmissions: %s", c.StateID, c.TransmissionsID)
	ticker := time.NewTicker(c.client.pollingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			c.lggr.Debugf("Stopping state polling for state: %s, transmissions: %s", c.StateID, c.TransmissionsID)
			return
		case <-ticker.C:
			// async poll both transmisison + ocr2 states
			go func() {
				ctx, cancel := utils.ContextFromChanWithDeadline(c.done, c.client.contextDuration)
				defer cancel()
				_, err, shared := c.requestGroup.Do("state", func() (interface{}, error) {
					return nil, c.fetchState(ctx)
				})
				if err != nil {
					c.lggr.Errorf("error in PollState.fetchState, shared: %t, error %s", shared, err)
				}
			}()
			go func() {
				ctx, cancel := utils.ContextFromChanWithDeadline(c.done, c.client.contextDuration)
				defer cancel()
				// make single flight request
				_, err, shared := c.requestGroup.Do("transmissions.latest", func() (interface{}, error) {
					return nil, c.fetchLatestTransmission(ctx)
				})
				if err != nil {
					c.lggr.Errorf("error in PollState.fetchLatestTransmission, shared: %t, error %s", shared, err)
				}
			}()
		}
	}
}

// Close stops the polling
func (c *ContractTracker) Close() error {
	return c.StopOnce("pollState", func() error {
		close(c.done)
		return nil
	})
}

// ReadState reads the latest state from memory with mutex and errors if timeout is exceeded
func (c *ContractTracker) ReadState() (State, solana.PublicKey, error) {
	c.stateLock.RLock()
	defer c.stateLock.RUnlock()

	var err error
	current := time.Now()
	if current.After(c.stateTime.Add(c.staleTimeout)) {
		err = errors.New("error in ReadState: stale state data, polling is likely experiencing errors")
	}
	return c.state, c.store, err
}

// ReadAnswer reads the latest state from memory with mutex and errors if timeout is exceeded
func (c *ContractTracker) ReadAnswer() (Answer, error) {
	c.ansLock.RLock()
	defer c.ansLock.RUnlock()

	// check if stale timeout
	var err error
	current := time.Now()
	if current.After(c.ansTime.Add(c.staleTimeout)) {
		err = errors.New("error in ReadAnswer: stale answer data, polling is likely experiencing errors")
	}
	return c.answer, err
}

// fetch + decode + store raw state
func (c *ContractTracker) fetchState(ctx context.Context) error {

	c.lggr.Debugf("fetch state for account: %s", c.StateID.String())
	state, _, err := GetState(ctx, c.client.rpc, c.StateID, c.client.commitment)
	if err != nil {
		return err
	}

	c.lggr.Debugf("state fetched for account: %s, result (config digest): %v", c.StateID, hex.EncodeToString(state.Config.LatestConfigDigest[:]))

	// Fetch the store address associated to the feed
	offset := uint64(8 + 1) // Discriminator (8 bytes) + Version (u8)
	length := uint64(solana.PublicKeyLength)
	res, err := c.client.rpc.GetAccountInfoWithOpts(ctx, state.Transmissions, &rpc.GetAccountInfoOpts{
		Encoding:   "base64",
		Commitment: c.client.commitment,
		DataSlice: &rpc.DataSlice{
			Offset: &offset,
			Length: &length,
		},
	})
	if err != nil {
		return err
	}

	// acquire lock and write to state
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	c.state = state
	c.store = solana.PublicKeyFromBytes(res.Value.Data.GetBinary())
	c.stateTime = time.Now()
	c.lggr.Debugf("store fetched for feed: %s", c.store)

	return nil
}

func (c *ContractTracker) fetchLatestTransmission(ctx context.Context) error {
	c.lggr.Debugf("fetch latest transmission for account: %s", c.TransmissionsID)
	answer, _, err := GetLatestTransmission(ctx, c.client.rpc, c.TransmissionsID, c.client.commitment)
	if err != nil {
		return err
	}
	c.lggr.Debugf("latest transmission fetched for account: %s, result: %v", c.TransmissionsID, answer)

	// acquire lock and write to state
	c.ansLock.Lock()
	defer c.ansLock.Unlock()
	c.answer = answer
	c.ansTime = time.Now()
	return nil
}

func GetState(ctx context.Context, client *rpc.Client, account solana.PublicKey, rpcCommitment rpc.CommitmentType) (State, uint64, error) {
	res, err := client.GetAccountInfoWithOpts(ctx, account, &rpc.GetAccountInfoOpts{
		Encoding:   "base64",
		Commitment: rpcCommitment,
	})
	if err != nil {
		return State{}, 0, fmt.Errorf("failed to fetch state account at address '%s': %w", account.String(), err)
	}

	var state State
	if err := bin.NewBinDecoder(res.Value.Data.GetBinary()).Decode(&state); err != nil {
		return State{}, 0, fmt.Errorf("failed to decode state account data: %w", err)
	}

	// validation for config version
	if configVersion != state.Version {
		return State{}, 0, fmt.Errorf("decoded config version (%d) does not match expected config version (%d)", state.Version, configVersion)
	}

	blockNum := res.RPCContext.Context.Slot
	return state, blockNum, nil
}

func GetLatestTransmission(ctx context.Context, client *rpc.Client, account solana.PublicKey, rpcCommitment rpc.CommitmentType) (Answer, uint64, error) {
	offset := CursorOffset
	length := CursorLen * 2
	transmissionLen := TransmissionLen

	// query for cursor
	res, err := client.GetAccountInfoWithOpts(ctx, account, &rpc.GetAccountInfoOpts{
		Encoding:   "base64",
		Commitment: rpcCommitment,
		DataSlice: &rpc.DataSlice{
			Offset: &offset,
			Length: &length,
		},
	})
	if err != nil {
		return Answer{}, 0, errors.Wrap(err, "error on rpc.GetAccountInfo [cursor]")
	}

	// parse little endian length & cursor
	c := res.Value.Data.GetBinary()
	buf := bytes.NewReader(c)

	var cursor uint32
	var liveLength uint32
	err = binary.Read(buf, binary.LittleEndian, &liveLength)
	if err != nil {
		return Answer{}, 0, errCursorLength
	}
	err = binary.Read(buf, binary.LittleEndian, &cursor)
	if err != nil {
		return Answer{}, 0, errCursorLength
	}

	if cursor == 0 { // handle array wrap
		cursor = liveLength
	}
	cursor-- // cursor indicates index for new answer, latest answer is in previous index

	// fetch transmission
	var transmissionOffset uint64 = 8 + 128 + (uint64(cursor) * transmissionLen)

	res, err = client.GetAccountInfoWithOpts(ctx, account, &rpc.GetAccountInfoOpts{
		Encoding:   "base64",
		Commitment: rpcCommitment,
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

	var timestamp uint64
	raw := make([]byte, 16)

	buf = bytes.NewReader(t)
	err = binary.Read(buf, binary.LittleEndian, &timestamp)
	if err != nil {
		return Answer{}, 0, err
	}

	// TODO: we could use ag_binary.Int128 instead
	err = binary.Read(buf, binary.LittleEndian, &raw)
	if err != nil {
		return Answer{}, 0, err
	}
	// reverse slice to change from little endian to big endian
	for i, j := 0, len(raw)-1; i < j; i, j = i+1, j-1 {
		raw[i], raw[j] = raw[j], raw[i]
	}

	return Answer{
		Data:      big.NewInt(0).SetBytes(raw[:]),
		Timestamp: timestamp,
	}, res.RPCContext.Context.Slot, nil
}
