package contracts

import _ "embed"

//go:embed target/idl/keystone-forwarder.json
var forwarderIdl string

// FetchCCIPRouterIDL returns
func FetchForwarderIDL() string {
	return forwarderIdl
}
