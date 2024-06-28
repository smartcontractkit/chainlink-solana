package solana

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-common/pkg/utils"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/monitor"
)

type TransmissionsCache struct {
	services.StateMachine

	// on-chain program + 2x state accounts (state + transmissions)
	TransmissionsID solana.PublicKey
	chainID         string

	ansLock sync.RWMutex
	answer  Answer
	ansTime time.Time

	// dependencies
	reader client.Reader
	cfg    config.Config
	lggr   logger.Logger

	// polling
	done   chan struct{}
	stopCh services.StopChan
}

func NewTransmissionsCache(transmissionsID solana.PublicKey, chainID string, cfg config.Config, reader client.Reader, lggr logger.Logger) *TransmissionsCache {
	return &TransmissionsCache{
		TransmissionsID: transmissionsID,
		chainID:         chainID,
		reader:          reader,
		lggr:            lggr,
		cfg:             cfg,
	}
}

// Start polling
func (c *TransmissionsCache) Start(ctx context.Context) error {
	return c.StartOnce("pollTransmissions", func() error {
		c.done = make(chan struct{})
		c.stopCh = make(chan struct{})
		// We synchronously update the config on start so that
		// when OCR starts there is config available (if possible).
		// Avoids confusing "contract has not been configured" OCR errors.
		err := c.fetchLatestTransmission(ctx)
		if err != nil {
			c.lggr.Warnf("error in initial PollTransmissions %s", err)
		}
		go c.PollTransmissions()
		return nil
	})
}

// Close stops the polling
func (c *TransmissionsCache) Close() error {
	return c.StopOnce("transmissionCache", func() error {
		close(c.stopCh)
		<-c.done
		return nil
	})
}

// PollTransmissions contains the transmissions polling implementation
func (c *TransmissionsCache) PollTransmissions() {
	defer close(c.done)
	ctx, cancel := c.stopCh.NewCtx()
	defer cancel()
	c.lggr.Debugf("Starting state polling transmissions: %s", c.TransmissionsID)
	tick := time.After(0)
	for {
		select {
		case <-ctx.Done():
			c.lggr.Debugf("Stopping state polling transmissions: %s", c.TransmissionsID)
			return
		case <-tick:
			// async poll both transmission + ocr2 states
			start := time.Now()
			err := c.fetchLatestTransmission(ctx)
			if err != nil {
				c.lggr.Errorf("error in PollTransmissions.fetchLatestTransmission %s", err)
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

	timestamp := time.Now()
	monitor.SetCacheTimestamp(timestamp, "ocr2_median_transmissions", c.chainID, c.TransmissionsID.String())
	// acquire lock and write to state
	c.ansLock.Lock()
	defer c.ansLock.Unlock()
	c.answer = answer
	c.ansTime = timestamp
	return nil
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
		return Answer{}, 0, fmt.Errorf("error on rpc.GetAccountInfo [cursor]: %w", err)
	}

	// check for nil pointers
	if res == nil || res.Value == nil || res.Value.Data == nil {
		return Answer{}, 0, errors.New("nil pointer returned in GetLatestTransmission.GetAccountInfoWithOpts.Header")
	}

	// parse header
	var header TransmissionsHeader
	if err = bin.NewBinDecoder(res.Value.Data.GetBinary()).Decode(&header); err != nil {
		return Answer{}, 0, fmt.Errorf("failed to decode transmission account header: %w", err)
	}

	if header.Version != 2 {
		return Answer{}, 0, fmt.Errorf("can't parse feed version %v: %w", header.Version, err)
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
		return Answer{}, 0, fmt.Errorf("error on rpc.GetAccountInfo [transmission]: %w", err)
	}
	// check for nil pointers
	if res == nil || res.Value == nil || res.Value.Data == nil {
		return Answer{}, 0, errors.New("nil pointer returned in GetLatestTransmission.GetAccountInfoWithOpts.Transmission")
	}

	// parse tranmission
	var t Transmission
	if err := bin.NewBinDecoder(res.Value.Data.GetBinary()).Decode(&t); err != nil {
		return Answer{}, 0, fmt.Errorf("failed to decode transmission: %w", err)
	}

	return Answer{
		Data:      t.Answer.BigInt(),
		Timestamp: t.Timestamp,
	}, res.RPCContext.Context.Slot, nil
}
