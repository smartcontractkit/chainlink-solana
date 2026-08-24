package fakes

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	solcap "github.com/smartcontractkit/chainlink-common/pkg/capabilities/v2/chain-capabilities/solana"
	sdk "github.com/smartcontractkit/chainlink-protos/cre/go/sdk"
	valuespb "github.com/smartcontractkit/chainlink-protos/cre/go/values/pb"
)

// accountToProto maps a solana-go *rpc.Account into the chain-capability proto
// Account and mirrors production's account-data conversion behavior closely.
func accountToProto(a *rpc.Account, pref solana.EncodingType) (*solcap.Account, error) {
	if a == nil {
		return nil, nil
	}
	data, err := convertDataBytesOrJSON(a.Data, pref)
	if err != nil {
		return nil, err
	}
	return &solcap.Account{
		Lamports:   a.Lamports,
		Owner:      a.Owner[:],
		Data:       data,
		Executable: a.Executable,
		RentEpoch:  valuespb.NewBigIntFromInt(a.RentEpoch),
		Space:      a.Space,
	}, nil
}

func convertGetAccountInfoOpts(opts *solcap.GetAccountInfoOpts) (*rpc.GetAccountInfoOpts, error) {
	if opts == nil {
		return &rpc.GetAccountInfoOpts{}, nil
	}

	out := &rpc.GetAccountInfoOpts{
		Encoding:       solana.EncodingType(defaultEncoding(opts.GetEncoding())),
		Commitment:     rpc.CommitmentType(defaultCommitment(opts.GetCommitment())),
		MinContextSlot: uint64Ptr(opts.GetMinContextSlot()),
	}
	if ds := opts.GetDataSlice(); ds != nil {
		out.DataSlice = &rpc.DataSlice{
			Offset: uint64Ptr(ds.GetOffset()),
			Length: uint64Ptr(ds.GetLength()),
		}
	}

	return out, nil
}

func convertDataBytesOrJSON(obj *rpc.DataBytesOrJSON, pref solana.EncodingType) (*solcap.DataBytesOrJSON, error) {
	if obj == nil {
		return nil, nil
	}
	if pref == "" {
		pref = solana.EncodingBase64
	}

	txBytes := obj.GetBinary()
	txJSON, jsonErr := json.Marshal(obj)
	if jsonErr != nil && len(txBytes) == 0 {
		return nil, fmt.Errorf("failed to marshal account data: %w", jsonErr)
	}

	switch pref {
	case solana.EncodingBase58, solana.EncodingBase64, solana.EncodingBase64Zstd:
		if len(txBytes) != 0 {
			return &solcap.DataBytesOrJSON{
				Encoding: encodingTypeToProto(pref),
				Body:     &solcap.DataBytesOrJSON_Raw{Raw: txBytes},
			}, nil
		}

		if pref != solana.EncodingBase64 {
			return nil, fmt.Errorf("expected binary account data for encoding %q but got empty bytes: %s", pref, truncateDiag(string(txJSON)))
		}

		var arr []string
		if err := json.Unmarshal(txJSON, &arr); err != nil {
			return nil, fmt.Errorf("expected base64 bytes but GetBinary() empty; also failed to parse json: %w json=%s", err, truncateDiag(string(txJSON)))
		}
		if len(arr) != 2 {
			return nil, fmt.Errorf("expected [data,encoding] json array, got len=%d json=%s", len(arr), truncateDiag(string(txJSON)))
		}
		if arr[1] != "base64" {
			return nil, fmt.Errorf("expected encoding base64, got %q json=%s", arr[1], truncateDiag(string(txJSON)))
		}

		b, err := base64.StdEncoding.DecodeString(arr[0])
		if err != nil {
			return nil, fmt.Errorf("base64 decode failed: %w", err)
		}
		return &solcap.DataBytesOrJSON{
			Encoding: solcap.EncodingType_ENCODING_TYPE_BASE64,
			Body:     &solcap.DataBytesOrJSON_Raw{Raw: b},
		}, nil

	case solana.EncodingJSON, solana.EncodingJSONParsed:
		return &solcap.DataBytesOrJSON{
			Encoding: encodingTypeToProto(pref),
			Body:     &solcap.DataBytesOrJSON_Json{Json: txJSON},
		}, nil

	default:
		if len(txBytes) == 0 {
			return nil, fmt.Errorf("expected binary account data but got empty bytes: %s", truncateDiag(string(txJSON)))
		}
		return &solcap.DataBytesOrJSON{
			Encoding: solcap.EncodingType_ENCODING_TYPE_BASE64,
			Body:     &solcap.DataBytesOrJSON_Raw{Raw: txBytes},
		}, nil
	}
}

func defaultEncoding(enc solcap.EncodingType) solana.EncodingType {
	switch enc {
	case solcap.EncodingType_ENCODING_TYPE_BASE58:
		return solana.EncodingBase58
	case solcap.EncodingType_ENCODING_TYPE_BASE64:
		return solana.EncodingBase64
	case solcap.EncodingType_ENCODING_TYPE_BASE64_ZSTD:
		return solana.EncodingBase64Zstd
	case solcap.EncodingType_ENCODING_TYPE_JSON_PARSED:
		return solana.EncodingJSONParsed
	case solcap.EncodingType_ENCODING_TYPE_JSON:
		return solana.EncodingJSON
	default:
		return solana.EncodingBase64
	}
}

func defaultCommitment(commitment solcap.CommitmentType) rpc.CommitmentType {
	switch commitment {
	case solcap.CommitmentType_COMMITMENT_TYPE_PROCESSED:
		return rpc.CommitmentProcessed
	case solcap.CommitmentType_COMMITMENT_TYPE_CONFIRMED:
		return rpc.CommitmentConfirmed
	default:
		return rpc.CommitmentFinalized
	}
}

func encodingTypeToProto(enc solana.EncodingType) solcap.EncodingType {
	switch enc {
	case solana.EncodingBase58:
		return solcap.EncodingType_ENCODING_TYPE_BASE58
	case solana.EncodingBase64:
		return solcap.EncodingType_ENCODING_TYPE_BASE64
	case solana.EncodingBase64Zstd:
		return solcap.EncodingType_ENCODING_TYPE_BASE64_ZSTD
	case solana.EncodingJSONParsed:
		return solcap.EncodingType_ENCODING_TYPE_JSON_PARSED
	case solana.EncodingJSON:
		return solcap.EncodingType_ENCODING_TYPE_JSON
	default:
		return solcap.EncodingType_ENCODING_TYPE_NONE
	}
}

func uint64Ptr(v uint64) *uint64 {
	return &v
}

const maxDiagPayloadLen = 1024

func truncateDiag(s string) string {
	if len(s) <= maxDiagPayloadLen {
		return s
	}
	return s[:maxDiagPayloadLen] + "...(truncated)"
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
