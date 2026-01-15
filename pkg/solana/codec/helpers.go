package codec

import (
	_ "embed"
)

//go:embed testutils/chainWriterTestIDL.json
var cwIDL string

// FetchCwIDL returns the IDL for chain components test contract
func FetchChainWriterTestIDL() string {
	return cwIDL
}

//go:embed testutils/logPollerTypeTestIDL.json
var logpollerTypeTestIDL string

// FetchLogpollerTypeTestIDL returns the IDL for logpoller type test
func FetchLogpollerTypeTestIDL() string {
	return logpollerTypeTestIDL
}
