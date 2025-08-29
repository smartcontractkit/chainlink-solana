package writetarget

import (
	"encoding/base64"
	"testing"

	"github.com/smartcontractkit/chainlink-common/pkg/capabilities"
	"github.com/smartcontractkit/chainlink-protos/cre/go/values"
	"github.com/stretchr/testify/require"
)

func b64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	require.NoError(t, err)
	return b
}

func Test_getRequest_UsingValuesWrap(t *testing.T) {
	const addrB58 = "Gokih6yWzpan2hqtayyVtpCjDQuDtEFBtSi5Kj9i4rAo"

	// --- Config (matches your log shape, but only the "address" field is used by getRequest)
	cfgGo := map[string]any{
		"address":    addrB58,
		"deltaStage": "1s",
		"params":     []any{"$(report)"},
		"schedule":   "oneAtATime",
	}
	cfgV, err := values.NewMap(cfgGo)
	require.NoError(t, err)

	// --- Inputs.signed_report
	signedReportGo := map[string]any{
		"Context":    b64(t, "AA76QcRshomVhUHuubDFXxPDbfwVsqt2r6NkdfEUgFQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"),
		"ID":         b64(t, "AAM="),
		"Report":     b64(t, "ATdBKZVEV/YFPrqaWR5DG5/sG//bwxvuAOdWlQk4xhU9aLEpuAAAAAEAAAABJZgzmz5jFMfe27YCPF3+78JO22tmsI8AW+KJOEvi+wR0ZXN0d2YAAAAAAQIDAAAAAAAAAAAAAAAAAAAAAAAAA+OwxEKY/BwUmvv0yJlvuSQnrkHkZJuTTKSVmRt4UrhVKAAAAAEAAAAKAAAADwAAAAAAAAAAAABQAAAAAAQBurj7YZfJ5wAAAAAAAAA="),
		"Signatures": []any{b64(t, "pTF7c5OHnk9NVOR65IObq1aZ1EhmfMLi4Pt3UvuFY0pJS22SLiYUpDkJbgkVFw287ruuWbz9u178jI/6osm3dQE="), b64(t, "+IKlHdiL10bIz7y2qW52uq900lhUlftJ+kbCuwo99aVS0pnVsmBLV//0mO4wAcrCUROMfYitFtI3YHojby17CgA=")},
	}

	signedReportV, err := values.NewMap(signedReportGo)
	require.NoError(t, err)

	// --- Inputs.remaining_accounts (use a minimal slice; add more if needed)
	remainingAccountsGo := []any{
		map[string]any{
			"PublicKey":  b64(t, "Ya2tXN1BLOUduwfHG81eZhmS5dzDujKDSFgFLAtMMOw="),
			"IsSigner":   false,
			"IsWritable": false,
		},
		map[string]any{
			"PublicKey":  b64(t, "RieaWjlZ7td4UI546Totaf6sCm1gn0guoT4YKi0Vd7g="),
			"IsSigner":   false,
			"IsWritable": false,
		},
		map[string]any{
			"PublicKey":  b64(t, "FlTgxp2ZWIQ4dIHcm+yxacXd9c24cm1pRpMxhASljgc="),
			"IsSigner":   false,
			"IsWritable": false,
		},
	}
	remainingAccountsV, err := values.Wrap(remainingAccountsGo)
	require.NoError(t, err)

	inputsV, err := values.NewMap(map[string]any{
		"signed_report":      signedReportV,      // key must match wt.KeySignedReport
		"remaining_accounts": remainingAccountsV, // key must match remainingAccountsKey
	})
	require.NoError(t, err)

	req := capabilities.CapabilityRequest{
		Config: cfgV,
		Inputs: inputsV,
	}

	got, err := getRequest(req)
	require.NoError(t, err)

	// Validate address -> Receiver
	require.Equal(t, addrB58, got.Config.Address)

	// Validate remaining accounts came through
	require.Len(t, got.Inputs.RemainingAccounts, 3)

	// Signed report present
	require.NotEmpty(t, got.Inputs.SignedReport.Report)
}
