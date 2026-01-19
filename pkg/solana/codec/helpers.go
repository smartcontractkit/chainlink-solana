package codec

import (
	_ "embed"
)

//go:embed testutils/itemArray1TypeIDL.json
var cwIDL string

// FetchCwIDL returns the IDL for chain components test contract
func FetchChainWriterTestIDL() string {
	return cwIDL
}

//go:embed testutils/eventItemTypeIDL.json
var logPollerTypeTestIDL string

// FetchLogpollerTypeTestIDL returns the IDL for log poller type test contract
func FetchLogpollerTypeTestIDL() string {
	return logPollerTypeTestIDL
}
