package ocr2

import (
	"errors"
	"time"
)

type Config struct {
	Smoke              *SmokeConfig `toml:"Smoke"`
	NodeCount          *int         `toml:"node_count"`
	NumberOfRounds     *int         `toml:"number_of_rounds"`
	TestDuration       *string      `toml:"test_duration"`
	TestDurationParsed *time.Duration
	Soak               *SoakConfig `toml:"Soak"`
}

type SoakConfig struct {
	Enabled           *bool   `toml:"enabled"`
	DetachRunner      *bool   `toml:"detach_runner"`
	RemoteRunnerImage *string `toml:"remote_runner_image"`
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

	if o.Smoke.Enabled == nil && o.Soak.Enabled == nil {
		return errors.New("OCR2.Smoke or OCR2.Soak must be defined")
	}

	err = o.Smoke.Validate()
	if err != nil {
		return err
	}

	err = o.Soak.Validate()
	if err != nil {
		return err
	}

	return nil
}

type SmokeConfig struct {
	Enabled *bool `toml:"enabled"`
}

func (o *SmokeConfig) Validate() error {
	if o.Enabled == nil {
		return errors.New("enabled must be set for OCR2.Smoke")
	}

	return nil
}

func (o *SoakConfig) Validate() error {
	if o.Enabled == nil {
		return errors.New("enabled must be set for OCR2.Soak")
	}

	if *o.Enabled {
		if o.RemoteRunnerImage == nil {
			return errors.New("remote_runner_image must be set for OCR2.Soak")
		}
		if o.DetachRunner == nil {
			return errors.New("detach_runner must be set for OCR2.Soak")
		}
	}

	return nil
}
