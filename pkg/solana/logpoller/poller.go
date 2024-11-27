package logpoller

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
)

type EventSaver interface {
	SaveEvent(evt ProgramEvent) error
}

type Service struct {
	// dependencies
	lggr  logger.Logger
	saver EventSaver

	// internal
	loader         *EncodedLogCollector
	mu             sync.RWMutex
	discriminators map[string]struct{}
	chSave         chan ProgramEvent

	// service state management
	services.Service
	engine *services.Engine
}

func New(client RPCClient, lggr logger.Logger, saver EventSaver) *Service {
	p := &Service{
		saver:          saver,
		discriminators: make(map[string]struct{}),
		chSave:         make(chan ProgramEvent),
	}

	p.Service, p.engine = services.Config{
		Name: "LogPollerService",
		NewSubServices: func(lggr logger.Logger) []services.Service {
			p.loader = NewEncodedLogCollector(client, p, lggr)

			return []services.Service{p.loader}
		},
		Start: p.start,
		Close: p.close,
	}.NewServiceEngine(lggr)
	p.lggr = p.engine.SugaredLogger

	return p
}

func (p *Service) AddFilter(name string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	hash := sha256.New()
	hash.Write([]byte(fmt.Sprintf("event:%s", name)))

	p.discriminators[string(hash.Sum(nil)[:8])] = struct{}{}

	return nil
}

func (p *Service) start(_ context.Context) error {
	p.engine.Go(p.runSaveProcess)

	return nil
}

func (p *Service) close() error {
	return nil
}

func (p *Service) Process(evt ProgramEvent) error {
	encodedData := strings.TrimSpace(evt.Data)
	data, err := base64.StdEncoding.DecodeString(encodedData)
	if err != nil {
		// don't return an error here, just log it
		// returning an error will trigger a retry
		p.lggr.Errorw("failed to base64 decode data", "err", err)

		return nil
	}

	// silently discard events that don't match any expected event signatures
	if !p.dataMatchesEventSig(data[:8]) {
		return nil
	}

	p.chSave <- evt

	return nil
}

func (p *Service) runSaveProcess(ctx context.Context) {
	// this process should ensure ordered batches before saving atomically to the database
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-p.chSave:
			if err := p.saver.SaveEvent(evt); err != nil {
				p.lggr.Errorw("failed to save event", "err", err)
			}
		}
	}
}

func (p *Service) dataMatchesEventSig(sig []byte) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	_, ok := p.discriminators[string(sig)]

	return ok
}
