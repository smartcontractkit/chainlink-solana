package ocr2

import (
	"errors"
	"time"
)

type Config struct {
	NodeCount          *int    `toml:"node_count"`
	NumberOfRounds     *int    `toml:"number_of_rounds"`
	TestDuration       *string `toml:"test_duration"`
	TestDurationParsed *time.Duration
}

func (o *Config) Validate() error {
	if o.NodeCount != nil && *o.NodeCount < 3 {
		return errors.New("node_count must be set and cannot be less than 3")
	}

	if o.TestDuration == nil {
		return errors.New("test_duration must be set")
	}
	duration, err := time.ParseDuration(*o.TestDuration)
	if err != nil {
		return errors.New("Invalid test duration")
	}
	o.TestDurationParsed = &duration

	if o.NumberOfRounds == nil {
		return errors.New("number_of_rounds must be set for OCR2")
	}

	return nil
}
