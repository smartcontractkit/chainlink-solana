package config

import (
	_ "embed"

	"github.com/smartcontractkit/chainlink-common/pkg/config/configdoc"
)

//go:embed docs.toml
var docsTOML string

//go:embed example.toml
var exampleConfig string

func GenerateDocs() (string, error) {
	return configdoc.Generate(docsTOML, `[//]: # (Documentation generated from docs.toml - DO NOT EDIT.)
This document describes the TOML format for configuration.`, exampleConfig, nil)
}
