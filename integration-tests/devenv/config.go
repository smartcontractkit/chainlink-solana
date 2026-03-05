package devenv

import (
	"fmt"
	"strings"
)

// DefaultSolanaCLNodeConfig produces a raw TOML config string for a Chainlink
// node configured for Solana. Uses a raw TOML template to avoid importing typed
// config structs from chainlink/v2 or chainlink-solana/pkg/solana/config.
func DefaultSolanaCLNodeConfig(chainID string, urls []string) (string, error) {
	var nodeEntries strings.Builder
	for i, u := range urls {
		fmt.Fprintf(&nodeEntries, "\n[[Solana.Nodes]]\nName = 'primary-%d'\nURL = '%s'\n", i, u)
	}

	return fmt.Sprintf(`[Log]
Level = 'debug'

[WebServer]
HTTPPort = 6688
SecureCookies = false
SessionTimeout = '999h0m0s'
[WebServer.TLS]
HTTPSPort = 0
[WebServer.RateLimit]
Authenticated = 2000
Unauthenticated = 100

[Feature]
FeedsManager = true
LogPoller = true
UICSAKeys = true

[OCR2]
Enabled = true

[P2P.V2]
Enabled = true
DeltaDial = '5s'
DeltaReconcile = '5s'
ListenAddresses = ['0.0.0.0:6690']

[[Solana]]
Enabled = true
ChainID = '%s'
[Solana.Chain]
TxTimeout = '2m0s'
[Solana.MultiNode]
Enabled = true
SyncThreshold = 170
VerifyChainID = false
%s`, chainID, nodeEntries.String()), nil
}
