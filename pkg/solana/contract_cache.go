package solana

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"go.uber.org/atomic"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/pkg/errors"
	"github.com/smartcontractkit/chainlink/core/utils"
)

var (
	configVersion       uint8 = 1
	defaultStaleTimeout       = 1 * time.Minute
	defaultPollInterval       = 1 * time.Second
)

type ContractCache struct {
	utils.StartStopOnce

	// on-chain program + 2x state accounts (state + transmissions)
	ProgramID       solana.PublicKey
	StateID         solana.PublicKey
	TransmissionsID solana.PublicKey
	StoreProgramID  solana.PublicKey

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

	// polling
	done   chan struct{}
	ctx    context.Context
	cancel context.CancelFunc

	// Signal contract config is found
	contractReady chan struct{}
	configFound   *atomic.Bool
}

func NewContractCache(spec OCR2Spec, client *Client, lggr Logger, contractReady chan struct{}) *ContractCache {
	// parse staleness timeout, if errors: use default timeout (1 min)
	staleTimeout, err := time.ParseDuration(spec.StaleTimeout)
	if err != nil {
		lggr.Warnf("could not parse stale timeout interval using default 1m")
		staleTimeout = defaultStaleTimeout
	}

	return &ContractCache{
		ProgramID:       spec.ProgramID,
		StateID:         spec.StateID,
		StoreProgramID:  spec.StoreProgramID,
		TransmissionsID: spec.TransmissionsID,
		client:          client,
		lggr:            lggr,
		stateLock:       &sync.RWMutex{},
		ansLock:         &sync.RWMutex{},
		staleTimeout:    staleTimeout,
		contractReady:   contractReady,
		configFound:     atomic.NewBool(false),
	}
}

// Start polling
func (c *ContractCache) Start() error {
	return c.StartOnce("pollState", func() error {
		c.done = make(chan struct{})
		ctx, cancel := context.WithCancel(context.Background())
		c.ctx = ctx
		c.cancel = cancel
		go c.PollState()
		return nil
	})
}

// PollState contains the state and transmissions polling implementation
func (c *ContractCache) PollState() {
	defer close(c.done)
	c.lggr.Debugf("Starting state polling for state: %s, transmissions: %s", c.StateID, c.TransmissionsID)
	tick := time.After(0)
	for {
		select {
		case <-c.ctx.Done():
			c.lggr.Debugf("Stopping state polling for state: %s, transmissions: %s", c.StateID, c.TransmissionsID)
			return
		case <-tick:
			// async poll both transmisison + ocr2 states
			start := time.Now()
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(c.ctx, c.client.contextDuration)
				defer cancel()
				err := c.fetchState(ctx)
				if err != nil {
					c.lggr.Errorf("error in PollState.fetchState %s", err)
				} else {
					// We have successfully read state from the contract
					// signal that we are ready to start libocr.
					// Only signal on the first successful fetch.
					if !c.configFound.Load() {
						close(c.contractReady)
						c.configFound.Store(true)
					}
				}
			}()
			go func() {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(c.ctx, c.client.contextDuration)
				defer cancel()
				err := c.fetchLatestTransmission(ctx)
				if err != nil {
					c.lggr.Errorf("error in PollState.fetchLatestTransmission %s", err)
				}
			}()
			wg.Wait()

			// Note negative duration will be immediately ready
			tick = time.After(utils.WithJitter(c.client.pollingInterval) - time.Since(start))
		}
	}
}

// Close stops the polling
func (c *ContractCache) Close() error {
	return c.StopOnce("pollState", func() error {
		c.cancel()
		<-c.done
		return nil
	})
}

// ReadState reads the latest state from memory with mutex and errors if timeout is exceeded
func (c *ContractCache) ReadState() (State, solana.PublicKey, error) {
	c.stateLock.RLock()
	defer c.stateLock.RUnlock()

	var err error
	if time.Since(c.stateTime) > c.staleTimeout {
		err = errors.New("error in ReadState: stale state data, polling is likely experiencing errors")
	}
	return c.state, c.store, err
}

// ReadAnswer reads the latest state from memory with mutex and errors if timeout is exceeded
func (c *ContractCache) ReadAnswer() (Answer, error) {
	c.ansLock.RLock()
	defer c.ansLock.RUnlock()

	// check if stale timeout
	var err error
	if time.Since(c.ansTime) > c.staleTimeout {
		err = errors.New("error in ReadAnswer: stale answer data, polling is likely experiencing errors")
	}
	return c.answer, err
}

// fetch + decode + store raw state
func (c *ContractCache) fetchState(ctx context.Context) error {

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

	store := solana.PublicKeyFromBytes(res.Value.Data.GetBinary())
	c.lggr.Debugf("store fetched for feed: %s", store)

	// acquire lock and write to state
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	c.state = state
	c.store = store
	c.stateTime = time.Now()
	return nil
}

func (c *ContractCache) fetchLatestTransmission(ctx context.Context) error {
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
	// query for transmission header
	var headerStart uint64 = 8 // skip account discriminator
	headerLen := HeaderLen
	res, err := client.GetAccountInfoWithOpts(ctx, account, &rpc.GetAccountInfoOpts{
		Encoding:   "base64",
		Commitment: rpcCommitment,
		DataSlice: &rpc.DataSlice{
			Offset: &headerStart,
			Length: &headerLen,
		},
	})
	if err != nil {
		return Answer{}, 0, errors.Wrap(err, "error on rpc.GetAccountInfo [cursor]")
	}

	// parse header
	var header TransmissionsHeader
	if err := bin.NewBinDecoder(res.Value.Data.GetBinary()).Decode(&header); err != nil {
		return Answer{}, 0, errors.Wrap(err, "failed to decode transmission account header")
	}

	cursor := header.LiveCursor
	liveLength := header.LiveLength

	if cursor == 0 { // handle array wrap
		cursor = liveLength
	}
	cursor-- // cursor indicates index for new answer, latest answer is in previous index

	// setup transmissionLen
	transmissionLen := TransmissionLen
	if header.Version == 1 {
		transmissionLen = TransmissionLenV1
	}

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

	// parse v1 transmission and return answer
	if header.Version == 1 {
		var t TransmissionV1
		if err := bin.NewBinDecoder(res.Value.Data.GetBinary()).Decode(&t); err != nil {
			return Answer{}, 0, errors.Wrap(err, "failed to decode v1 transmission")
		}

		return Answer{
			Data:      t.Answer.BigInt(),
			Timestamp: uint32(t.Timestamp), // TODO: not good typing conversion
		}, res.RPCContext.Context.Slot, nil
	}

	// parse tranmission
	var t Transmission
	if err := bin.NewBinDecoder(res.Value.Data.GetBinary()).Decode(&t); err != nil {
		return Answer{}, 0, errors.Wrap(err, "failed to decode transmission")
	}

	return Answer{
		Data:      t.Answer.BigInt(),
		Timestamp: t.Timestamp,
	}, res.RPCContext.Context.Slot, nil
}
