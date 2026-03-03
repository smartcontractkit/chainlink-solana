package codecv1

import (
	_ "embed"
)

//go:embed testutils/chainWriterTestIDL.json
var cwIDL string

// FetchCwIDL returns the IDL for chain components test contract
func FetchChainWriterTestIDL() string {
	return cwIDL
}
