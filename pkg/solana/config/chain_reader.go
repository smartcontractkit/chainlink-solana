package config

import "github.com/smartcontractkit/chainlink-common/pkg/codec"

type ChainReader struct {
	Namespaces map[string]ChainReaderMethods `json:"namespaces" toml:"namespaces"`
}

type ChainReaderMethods struct {
	Methods map[string]ChainDataReader `json:"methods" toml:"methods"`
}

type ChainDataReader struct {
	AnchorIDL  string                 `json:"anchorIDL" toml:"anchorIDL"`
	Procedures []ChainReaderProcedure `json:"procedures" toml:"procedures"`
}

type ProcedureType int

const (
	ProcedureTypeInternal ProcedureType = iota
	ProcedureTypeAnchor
)

type ChainReaderProcedure chainDataProcedureFields

type chainDataProcedureFields struct {
	// IDLAccount refers to the account defined in the IDL.
	IDLAccount string `json:"idlAccount"`
	// Type describes the procedure type to use such as internal for static values,
	// anchor-read for using an anchor generated IDL to read values from an account,
	// or custom structure for reading from a native account.
	Type ProcedureType `json:"type"`
	// OutputModifications provides modifiers to convert chain data format to custom
	// output formats.
	OutputModifications codec.ModifiersConfig `json:"outputModifications,omitempty"`
}
