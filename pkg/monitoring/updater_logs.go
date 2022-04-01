package monitoring

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"regexp"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc/ws"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
)

func NewLogsUpdater(
	client *ws.Client,
	program solana.PublicKey,
	log relayMonitoring.Logger,
) Updater {
	return &logsUpdater{
		client,
		program,
		make(chan interface{}),
		log,
	}
}

type logsUpdater struct {
	client  *ws.Client
	program solana.PublicKey
	updates chan interface{}
	log     relayMonitoring.Logger
}

func (l *logsUpdater) Run(ctx context.Context) {
SUBSCRIBE_LOOP:
	for {
		subscription, err := l.client.LogsSubscribeMentions(l.program, commitment)
		if err != nil {
			l.log.Errorw("error creating logs subscription, retrying: %w", err)
			// TODO (dru) a better reconnect logic: exp backoff, error-specific handling.
			continue SUBSCRIBE_LOOP
		}
	RECEIVE_LOOP:
		for {
			result, err := subscription.Recv()
			if err != nil {
				l.log.Errorw("error reading message from subscription, reconnecting: %w", err)
				subscription.Unsubscribe()
				continue SUBSCRIBE_LOOP
			}
			log := Log{
				Slot:      result.Context.Slot,
				Signature: result.Value.Signature[:],
				Err:       result.Value.Err,
				Logs:      filterAndDeserializeLogs(result.Value.Logs, program),
			}
			select {
			case l.updates <- log:
			case <-ctx.Done():
				subscription.Unsubscribe()
				return
			}
		}
	}
}

func (l *logsUpdater) Updates() <-chan interface{} {
	return l.updates
}

// Helpers

var programInvocation = regexp.MustCompile("^Program\\s([a-zA-Z0-9])+?\\sinvoke\\s[\\d]$")
var programFinish = regexp.MustCompile("^Program\\s([a-zA-Z0-9])+?\\s(success|error)$")
var programLogEvent = regexp.MustCompile("^Program\\s(log|data):\\s([+/0-9A-Za-z]+={0,2})?$")

func filterLogs(logs []string, programID string) []string {
	invocationStack := []string{}
	output := []string{}
	for _, log := range logs {
		if matches := programInvocation.FindStringSubmatch(log); matches != nil {
			invokedProgramID := matches[1]
			invocationStack = append(invocationStack, invokedProgramID)
		} else if matches := programFinished.FindStringSubmatch(log); matches != nil {
			finishedProgramID := matches[1]
			if invocationStack[len(invocationStack)-1] != finishedProgramID {
				// Oh noes!
			} else {
				invocationStack = invocationStack[:len(invocationStack)-1]
			}
		} else if matches := programLogEvent.FindStringSubmatch(log); matches != nil {
			currentProgramID := invocationStack[len(invocationStack)-1]
			if programID == currentProgramID {
				output = append(output, matches[1])
			}
		}
	}
	return output
}

func deserilizeLogs(logs []string) ([]interface{}, error) {
	for _, log := range logs {
		buf, err := base64.StdEncoding.DecodeString(log)
		if err != nil {
			return nil, err
		}
		switch buf[:8] {
		case SetConfigDiscriminator:
		case SetBillingDiscriminator:
		case RoundRequestedDiscriminator:
		case NewTransmissionDiscriminator:
		}
		bin.NewDecoder(buf).Decode
	}
}

var (
	SetConfigDiscriminator       []byte
	SetBillingDiscriminator      []byte
	RoundRequestedDiscriminator  []byte
	NewTransmissionDiscriminator []byte
)

func init() {
	SetConfigDiscriminator = sha256.Sum256(fmt.Sprintf("event:SetConfig"))[:8]
	SetBillingDiscriminator = sha256.Sum256(fmt.Sprintf("event:SetBilling"))[:8]
	RoundRequestedDiscriminator = sha256.Sum256(fmt.Sprintf("event:RoundRequested"))[:8]
	NewTransmissionDiscriminator = sha256.Sum256(fmt.Sprintf("event:NewTransmission"))[:8]
}

type SetConfig struct {
	ConfigDigest [32]uint8   `json:"config_digest,omitempty"`
	F            uint8       `json:"f,omitempty"`
	Signers      [][20]uint8 `json:"signers,omitempty"`
}

type SetBilling struct {
	ObservationPaymentGJuels  uint32 `json:"observation_payment_gjuels,omitempty"`
	TransmissionPaymentGJuels uint32 `json:"transmission_payment_gjuels,omitempty"`
}

type RoundRequested struct {
	ConfigDigest [32]uint8        `json:"config_digest,omitempty"`
	Requester    solana.PublicKey `json:"requester,omitempty"`
	Epoch        uint32           `json:"epoch,omitempty"`
	Round        uint8            `json:"round,omitempty"`
}

type NewTransmission struct {
	RoundID               uint32     `json:"round_id,omitempty"`
	ConfigDigest          [32]uint8  `json:"config_digest,omitempty"`
	Answer                bin.Int128 `json:"answer,omitempty"`
	Transmitter           uint8      `json:"transmitter,omitempty"`
	ObservationsTimestamp uint32     `json:"observations_timestamp,omitempty"`
	ObserverCount         uint8      `json:"observer_count,omitempty"`
	Observers             [19]uint8  `json:"observers,omitempty"`
	JuelsPerLamport       uint64     `json:"juels_per_lamport,omitempty"`
	ReimbursementGJuels   uint64     `json:"reimbursement_gjuels,omitempty"`
}
