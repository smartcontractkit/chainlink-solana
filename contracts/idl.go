package contracts

import _ "embed"

//go:embed idl/keystone_forwarder.json
var forwarderIdl string

// FetchCCIPRouterIDL returns
func FetchForwarderIDL() string {
	return forwarderIdl
}
