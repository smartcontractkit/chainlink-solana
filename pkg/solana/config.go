package solana

import "net/url"

// Required is an array of env vars that must be set for each config type
// "ETH_HTTP_URL" is part of the core chainlink config, but may not be required for every relay
var Required = []string{"ETH_HTTP_URL", "ETH_WS_URL"}

// Core is the required configs that are collected by chainlink core
type CoreConfig interface {
	EthereumHTTPURL() *url.URL
	EthereumURL() string
}
