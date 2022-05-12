package solana

import (
	"context"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/pkg/errors"
	"github.com/smartcontractkit/chainlink/core/utils"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logger"
)

type TransmissionsCache struct {
	// on-chain program + 2x state accounts (state + transmissions)
	ProgramID       solana.PublicKey
	StateID         solana.PublicKey
	TransmissionsID solana.PublicKey
	StoreProgramID  solana.PublicKey

	// private key for the transmission signing
	transmitterSet bool
	Transmitter    TransmissionSigner

	// tracked contract state
	//state  State
	answer Answer

	// read/write mutexes
	//stateLock *sync.RWMutex
	ansLock *sync.RWMutex

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

func NewTransmissionsCache(programID, stateID, storeProgramID, transmissionsID solana.PublicKey, cfg config.Config, reader client.Reader, txManager TxManager, transmitter TransmissionSigner, lggr logger.Logger) TransmissionsCache {
	return TransmissionsCache{
		ProgramID:       programID,
		StateID:         stateID,
		StoreProgramID:  storeProgramID,
		TransmissionsID: transmissionsID,
		Transmitter:     transmitter,
		reader:          reader,
		txManager:       txManager,
		lggr:            lggr,
		cfg:             cfg,
		//stateLock:       &sync.RWMutex{},
		ansLock: &sync.RWMutex{},
	}
}

// Start polling
func (c *TransmissionsCache) Start() error {
	return c.StartOnce("pollState", func() error {
		c.done = make(chan struct{})
		ctx, cancel := context.WithCancel(context.Background())
		c.ctx = ctx
		c.cancel = cancel
		// We synchronously update the config on start so that
		// when OCR starts there is config available (if possible).
		// Avoids confusing "contract has not been configured" OCR errors.
		err := c.fetchLatestTransmission(c.ctx)
		if err != nil {
			c.lggr.Warnf("error in initial PollState.fetchState %s", err)
		}
		go c.PollTransmissions()
		return nil
	})
}

// PollState contains the transmissions polling implementation
func (c *TransmissionsCache) PollTransmissions() {
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
			err := c.fetchLatestTransmission(c.ctx)
			if err != nil {
				c.lggr.Errorf("error in PollState.fetchLatestTransmission %s", err)
			}
			// Note negative duration will be immediately ready
			tick = time.After(utils.WithJitter(c.cfg.OCR2CachePollPeriod()) - time.Since(start))
		}
	}
}

// ReadAnswer reads the latest state from memory with mutex and errors if timeout is exceeded
func (c *TransmissionsCache) ReadAnswer() (Answer, error) {
	c.ansLock.RLock()
	defer c.ansLock.RUnlock()

	// check if stale timeout
	var err error
	if time.Since(c.ansTime) > c.cfg.OCR2CacheTTL() {
		err = errors.New("error in ReadAnswer: stale answer data, polling is likely experiencing errors")
	}
	return c.answer, err
}

func (c *TransmissionsCache) fetchLatestTransmission(ctx context.Context) error {
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
