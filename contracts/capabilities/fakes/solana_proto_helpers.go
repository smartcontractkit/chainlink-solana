package fakes

import (
	"bytes"
	"crypto/sha256"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	solcap "github.com/smartcontractkit/chainlink-common/pkg/capabilities/v2/chain-capabilities/solana"
	sdk "github.com/smartcontractkit/chainlink-protos/cre/go/sdk"
	valuespb "github.com/smartcontractkit/chainlink-protos/cre/go/values/pb"
)

// accountToProto maps a solana-go *rpc.Account into the chain-capability proto Account.
// Subset of fields — only what GetAccountInfoWithOptsReply consumers commonly read.
// Skips Data because the proto's DataBytesOrJSON requires encoding-type negotiation
// that no current cre-cli simulation consumer needs (canary doesn't call reads).
// Backfill the Data field when a workflow exercises it.
func accountToProto(a *rpc.Account) *solcap.Account {
	if a == nil {
		return nil
	}
	return &solcap.Account{
		Lamports:   a.Lamports,
		Owner:      a.Owner[:],
		Executable: a.Executable,
		RentEpoch:  valuespb.NewBigIntFromInt(a.RentEpoch),
		Space:      a.Space,
	}
}

// buildReportPayload assembles the forwarder `data` arg:
// data = len_sigs (1) | signatures (N*65) | raw_report (M) | report_context (96)
// Mirrors capabilities/chain_capabilities/solana/actions/forwarder_client.go::toPayload.
func buildReportPayload(report *sdk.ReportResponse) ([]byte, error) {
	if len(report.Sigs) > maxOracles {
		return nil, fmt.Errorf("signature count %d exceeds max %d", len(report.Sigs), maxOracles)
	}
	for i, sig := range report.Sigs {
		if sig == nil || len(sig.Signature) != signatureLen {
			return nil, fmt.Errorf("signature %d invalid length", i)
		}
	}
	if len(report.ReportContext) != reportContextLen {
		return nil, fmt.Errorf("report context length %d, want %d", len(report.ReportContext), reportContextLen)
	}

	out := make([]byte, 0, 1+len(report.Sigs)*signatureLen+len(report.RawReport)+reportContextLen)
	out = append(out, byte(len(report.Sigs)))
	for _, sig := range report.Sigs {
		out = append(out, sig.Signature...)
	}
	out = append(out, report.RawReport...)
	out = append(out, report.ReportContext...)
	return out, nil
}

// patchReportAccountHash overwrites ForwarderReport.account_hash inside an
// assembled report payload with sha256 over the account list the mock
// forwarder rebuilds on-chain: [state, authority, ...receiverAccounts].
// The hash sits at raw_report[reportMetadataLen : reportMetadataLen+sha256.Size],
// and raw_report starts after len_sigs(1) + signatures(numSigs*65).
// Same digest the SDK bindings produce (cre-sdk-go
// capabilities/blockchain/solana/bindings CalculateAccountsHash) — keep in sync.
// Returns whether the hash actually differed from the workflow-computed one.
func patchReportAccountHash(payload []byte, numSigs int, state, authority solana.PublicKey, receiverAccounts []*solana.AccountMeta) (bool, error) {
	start := 1 + numSigs*signatureLen + reportMetadataLen
	end := start + sha256.Size
	if end > len(payload)-reportContextLen {
		return false, fmt.Errorf("report payload too short to carry a ForwarderReport account hash (len %d, need raw report >= %d)", len(payload), reportMetadataLen+sha256.Size)
	}

	h := sha256.New()
	h.Write(state.Bytes())
	h.Write(authority.Bytes())
	for _, acc := range receiverAccounts {
		h.Write(acc.PublicKey.Bytes())
	}
	sum := h.Sum(nil)

	if bytes.Equal(payload[start:end], sum) {
		return false, nil
	}
	copy(payload[start:end], sum)
	return true, nil
}
