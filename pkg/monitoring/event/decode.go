package event

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"regexp"
)

var programInvocation = regexp.MustCompile(`^Program\s([a-zA-Z0-9]+)?\sinvoke\s\[\d\]$`)
var programFinished = regexp.MustCompile(`^Program\s([a-zA-Z0-9]+)?\s(?:success|error)$`)
var programLogEvent = regexp.MustCompile(`^Program\s(?:log|data):\s([+/0-9A-Za-z]+={0,2})?$`)

func ExtractEvents(logs []string, programIDBase58 string) []string {
	invocationStack := []string{}
	output := []string{}
	for _, log := range logs {
		if matches := programInvocation.FindStringSubmatch(log); matches != nil {
			invokedProgramID := matches[1]
			invocationStack = append(invocationStack, invokedProgramID)
			continue
		}
		if matches := programLogEvent.FindStringSubmatch(log); matches != nil {
			currentProgramID := invocationStack[len(invocationStack)-1]
			if programIDBase58 == currentProgramID {
				output = append(output, matches[1])
			}
			continue
		}
		if matches := programFinished.FindStringSubmatch(log); matches != nil {
			if len(invocationStack) == 0 {
				break // incorrect execution trace.
			}
			finishedProgramID := matches[1]
			if invocationStack[len(invocationStack)-1] == finishedProgramID {
				invocationStack = invocationStack[:len(invocationStack)-1]
			}
		}
	}
	return output
}

// Decode extracts an event from the encoded event given as a string.
func Decode(base64Encoded string) (interface{}, error) {
	buf, err := base64.StdEncoding.DecodeString(base64Encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode event '%s' from base64: %w", base64Encoded, err)
	}
	if len(buf) < discriminatorLength {
		return nil, fmt.Errorf("expected event data to have at least %d bytes, but had %d", discriminatorLength, len(buf))
	}
	discriminator, encoded := buf[:discriminatorLength], buf[discriminatorLength:]
	switch true {
	case bytes.Equal(discriminator, SetConfigDiscriminator):
		var event SetConfig
		if err = event.UnmarshalBinary(encoded); err != nil {
			return nil, fmt.Errorf("failed to decode event '%v' of type '%T': %w", encoded, event, err)
		}
		return event, nil
	case bytes.Equal(discriminator, SetBillingDiscriminator):
		var event SetBilling
		if err = event.UnmarshalBinary(encoded); err != nil {
			return nil, fmt.Errorf("failed to decode event '%v' of type '%T': %w", encoded, event, err)
		}
		return event, nil
	case bytes.Equal(discriminator, RoundRequestedDiscriminator):
		var event RoundRequested
		if err = event.UnmarshalBinary(encoded); err != nil {
			return nil, fmt.Errorf("failed to decode event '%v' of type '%T': %w", encoded, event, err)
		}
		return event, nil
	case bytes.Equal(discriminator, NewTransmissionDiscriminator):
		var event NewTransmission
		if err = event.UnmarshalBinary(encoded); err != nil {
			return nil, fmt.Errorf("failed to decode event '%v' of type '%T': %w", encoded, event, err)
		}
		return event, nil
	}
	return nil, fmt.Errorf("Unrecognised event discriminator %x", discriminator)
}

func DecodeMultiple(base64EncodedEvents []string) ([]interface{}, error) {
	events := []interface{}{}
	for _, encoded := range base64EncodedEvents {
		event, err := Decode(encoded)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

const discriminatorLength = 8

func getDiscriminator(prefix string) []byte {
	hash := sha256.Sum256([]byte(prefix))
	return hash[:discriminatorLength]
}
