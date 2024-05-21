package ocr2

import (
	"errors"
	"time"
)

type Config struct {
	Smoke              *SmokeConfig `toml:"Smoke"`
	NodeCount          *int         `toml:"node_count"`
	TestDuration       *string      `toml:"test_duration"`
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

	if o.Smoke == nil {
		return errors.New("smoke must be defined")
	}
	err = o.Smoke.Validate()
	if err != nil {
		return err
	}

	return nil
}

type SmokeConfig struct {
	NumberOfRounds *int `toml:"number_of_rounds"`
}

func (o *SmokeConfig) Validate() error {
	if o.NumberOfRounds == nil {
		return errors.New("number_of_rounds must be set")
	}
	return nil
}
