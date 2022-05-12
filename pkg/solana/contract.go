package solana

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/pkg/errors"
	"github.com/smartcontractkit/chainlink/core/utils"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logger"
)

var (
	configVersion uint8 = 1
)

type ContractTracker struct {
	// on-chain program + 2x state accounts (state + transmissions)
	ProgramID       solana.PublicKey
	StateID         solana.PublicKey
	TransmissionsID solana.PublicKey
	StoreProgramID  solana.PublicKey

	// private key for the transmission signing
	transmitterSet bool
	Transmitter    TransmissionSigner

	// tracked contract state
	state  State
	answer Answer

	// read/write mutexes
	stateLock *sync.RWMutex
	ansLock   *sync.RWMutex

	// stale state parameters
	stateTime time.Time
	ansTime   time.Time

	// dependencies
	reader    client.Reader
	txManager TxManager
	cfg       config.Config
	lggr      logger.Logger

	// polling
	done   chan struct{}
	ctx    context.Context
	cancel context.CancelFunc

	utils.StartStopOnce
}

func NewTracker(programID, stateID, storeProgramID, transmissionsID solana.PublicKey, cfg config.Config, reader client.Reader, txManager TxManager, transmitter TransmissionSigner, lggr logger.Logger) ContractTracker {
	return ContractTracker{
		ProgramID:       programID,
		StateID:         stateID,
		StoreProgramID:  storeProgramID,
		TransmissionsID: transmissionsID,
		Transmitter:     transmitter,
		reader:          reader,
		txManager:       txManager,
		lggr:            lggr,
		cfg:             cfg,
		stateLock:       &sync.RWMutex{},
		ansLock:         &sync.RWMutex{},
	}
}

// Start polling
func (c *ContractTracker) Start() error {
	return c.StartOnce("pollState", func() error {
		c.done = make(chan struct{})
		ctx, cancel := context.WithCancel(context.Background())
		c.ctx = ctx
		c.cancel = cancel
		// We synchronously update the config on start so that
		// when OCR starts there is config available (if possible).
		// Avoids confusing "contract has not been configured" OCR errors.
		err := c.fetchState(c.ctx)
		if err != nil {
			c.lggr.Warnf("error in initial PollState.fetchState %s", err)
		}
		go c.PollState()
		return nil
	})
}

// PollState contains the state and transmissions polling implementation
func (c *ContractTracker) PollState() {
	defer close(c.done)
	c.lggr.Debugf("Starting state polling for state: %s, transmissions: %s", c.StateID, c.TransmissionsID)
	tick := time.After(0)
	for {
		select {
		case <-c.ctx.Done():
			c.lggr.Debugf("Stopping state polling for state: %s, transmissions: %s", c.StateID, c.TransmissionsID)
			return
		case <-tick:
			// async poll both transmission + ocr2 states
			start := time.Now()
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				err := c.fetchState(c.ctx)
				if err != nil {
					c.lggr.Errorf("error in PollState.fetchState %s", err)
				}
			}()
			go func() {
				defer wg.Done()
				err := c.fetchLatestTransmission(c.ctx)
				if err != nil {
					c.lggr.Errorf("error in PollState.fetchLatestTransmission %s", err)
				}
			}()
			wg.Wait()

			// Note negative duration will be immediately ready
			tick = time.After(utils.WithJitter(c.cfg.OCR2CachePollPeriod()) - time.Since(start))
		}
	}
}

// Close stops the polling
func (c *ContractTracker) Close() error {
	return c.StopOnce("pollState", func() error {
		c.cancel()
		<-c.done
		return nil
	})
}

// ReadState reads the latest state from memory with mutex and errors if timeout is exceeded
func (c *ContractTracker) ReadState() (State, error) {
	c.stateLock.RLock()
	defer c.stateLock.RUnlock()

	var err error
	if time.Since(c.stateTime) > c.cfg.OCR2CacheTTL() {
		err = errors.New("error in ReadState: stale state data, polling is likely experiencing errors")
	}
	return c.state, err
}

// ReadAnswer reads the latest state from memory with mutex and errors if timeout is exceeded
func (c *ContractTracker) ReadAnswer() (Answer, error) {
	c.ansLock.RLock()
	defer c.ansLock.RUnlock()

	// check if stale timeout
	var err error
	if time.Since(c.ansTime) > c.cfg.OCR2CacheTTL() {
		err = errors.New("error in ReadAnswer: stale answer data, polling is likely experiencing errors")
	}
	return c.answer, err
}

// fetch + decode + store raw state
func (c *ContractTracker) fetchState(ctx context.Context) error {

	c.lggr.Debugf("fetch state for account: %s", c.StateID.String())
	state, _, err := GetState(ctx, c.reader, c.StateID, c.cfg.Commitment())
	if err != nil {
		return err
	}

	c.lggr.Debugf("state fetched for account: %s, result (config digest): %v", c.StateID, hex.EncodeToString(state.Config.LatestConfigDigest[:]))

	// acquire lock and write to state
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	c.state = state
	c.stateTime = time.Now()
	return nil
}

func (c *ContractTracker) fetchLatestTransmission(ctx context.Context) error {
	c.lggr.Debugf("fetch latest transmission for account: %s", c.TransmissionsID)
	answer, _, err := GetLatestTransmission(ctx, c.reader, c.TransmissionsID, c.cfg.Commitment())
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

func GetState(ctx context.Context, reader client.AccountReader, account solana.PublicKey, commitment rpc.CommitmentType) (State, uint64, error) {
	res, err := reader.GetAccountInfoWithOpts(ctx, account, &rpc.GetAccountInfoOpts{
		Commitment: commitment,
		Encoding:   "base64",
	})
	if err != nil {
		return State{}, 0, fmt.Errorf("failed to fetch state account at address '%s': %w", account.String(), err)
	}

	// check for nil pointers
	if res == nil || res.Value == nil || res.Value.Data == nil {
		return State{}, 0, errors.New("nil pointer returned in GetState.GetAccountInfoWithOpts")
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

func GetLatestTransmission(ctx context.Context, reader client.AccountReader, account solana.PublicKey, commitment rpc.CommitmentType) (Answer, uint64, error) {
	// query for transmission header
	headerStart := AccountDiscriminatorLen // skip account discriminator
	headerLen := TransmissionsHeaderLen
	res, err := reader.GetAccountInfoWithOpts(ctx, account, &rpc.GetAccountInfoOpts{
		Encoding:   "base64",
		Commitment: commitment,
		DataSlice: &rpc.DataSlice{
			Offset: &headerStart,
			Length: &headerLen,
		},
	})
	if err != nil {
		return Answer{}, 0, errors.Wrap(err, "error on rpc.GetAccountInfo [cursor]")
	}

	// check for nil pointers
	if res == nil || res.Value == nil || res.Value.Data == nil {
		return Answer{}, 0, errors.New("nil pointer returned in GetLatestTransmission.GetAccountInfoWithOpts.Header")
	}

	// parse header
	var header TransmissionsHeader
	if err = bin.NewBinDecoder(res.Value.Data.GetBinary()).Decode(&header); err != nil {
		return Answer{}, 0, errors.Wrap(err, "failed to decode transmission account header")
	}

	if header.Version != 2 {
		return Answer{}, 0, errors.Wrapf(err, "can't parse feed version %v", header.Version)
	}

	cursor := header.LiveCursor
	liveLength := header.LiveLength

	if cursor == 0 { // handle array wrap
		cursor = liveLength
	}
	cursor-- // cursor indicates index for new answer, latest answer is in previous index

	// setup transmissionLen
	transmissionLen := TransmissionLen

	transmissionOffset := AccountDiscriminatorLen + TransmissionsHeaderMaxSize + (uint64(cursor) * transmissionLen)

	res, err = reader.GetAccountInfoWithOpts(ctx, account, &rpc.GetAccountInfoOpts{
		Encoding:   "base64",
		Commitment: commitment,
		DataSlice: &rpc.DataSlice{
			Offset: &transmissionOffset,
			Length: &transmissionLen,
		},
	})
	if err != nil {
		return Answer{}, 0, errors.Wrap(err, "error on rpc.GetAccountInfo [transmission]")
	}
	// check for nil pointers
	if res == nil || res.Value == nil || res.Value.Data == nil {
		return Answer{}, 0, errors.New("nil pointer returned in GetLatestTransmission.GetAccountInfoWithOpts.Transmission")
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
