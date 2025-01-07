package utils

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/internal"
)

var (
	_, b, _, _ = runtime.Caller(0)
	// ProjectRoot Root folder of this project
	ProjectRoot = filepath.Join(filepath.Dir(b), "/../../..")
	// ContractsDir path to our contracts
	ContractsDir       = filepath.Join(ProjectRoot, "contracts", "target", "deploy")
	PathToAnchorConfig = filepath.Join(ProjectRoot, "contracts", "Anchor.toml")
)

func LamportsToSol(lamports uint64) float64 { return internal.LamportsToSol(lamports) }

// TxModifier is a dynamic function used to flexibly add components to a transaction such as additional signers, and compute budget parameters
type TxModifier func(tx *solana.Transaction, signers map[solana.PublicKey]solana.PrivateKey) error

func SendAndConfirm(ctx context.Context, t *testing.T, rpcClient *rpc.Client, instructions []solana.Instruction,
	signer solana.PrivateKey, commitment rpc.CommitmentType, opts ...TxModifier) *rpc.GetTransactionResult {
	txres := sendTransaction(ctx, rpcClient, t, instructions, signer, commitment, false, opts...) // do not skipPreflight when expected to pass, preflight can help debug

	require.NotNil(t, txres.Meta)
	require.Nil(t, txres.Meta.Err, fmt.Sprintf("tx failed with: %+v", txres.Meta)) // tx should not err, print meta if it does (contains logs)
	return txres
}

func sendTransaction(ctx context.Context, rpcClient *rpc.Client, t *testing.T, instructions []solana.Instruction,
	signerAndPayer solana.PrivateKey, commitment rpc.CommitmentType, skipPreflight bool, opts ...TxModifier) *rpc.GetTransactionResult {
	hashRes, err := rpcClient.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	require.NoError(t, err)

	tx, err := solana.NewTransaction(
		instructions,
		hashRes.Value.Blockhash,
		solana.TransactionPayer(signerAndPayer.PublicKey()),
	)
	require.NoError(t, err)

	// build signers map
	signers := map[solana.PublicKey]solana.PrivateKey{}
	signers[signerAndPayer.PublicKey()] = signerAndPayer

	// set options before signing transaction
	for _, o := range opts {
		require.NoError(t, o(tx, signers))
	}

	_, err = tx.Sign(func(pub solana.PublicKey) *solana.PrivateKey {
		priv, ok := signers[pub]
		require.True(t, ok, fmt.Sprintf("Missing signer private key for %s", pub))
		return &priv
	})
	require.NoError(t, err)

	txsig, err := rpcClient.SendTransactionWithOpts(ctx, tx, rpc.TransactionOpts{SkipPreflight: skipPreflight, PreflightCommitment: rpc.CommitmentProcessed})
	require.NoError(t, err)

	var txStatus rpc.ConfirmationStatusType
	count := 0
	for txStatus != rpc.ConfirmationStatusType(commitment) && txStatus != rpc.ConfirmationStatusFinalized {
		count++
		statusRes, sigErr := rpcClient.GetSignatureStatuses(ctx, true, txsig)
		require.NoError(t, sigErr)
		if statusRes != nil && len(statusRes.Value) > 0 && statusRes.Value[0] != nil {
			txStatus = statusRes.Value[0].ConfirmationStatus
		}
		time.Sleep(100 * time.Millisecond)
		if count > 500 {
			require.NoError(t, fmt.Errorf("unable to find transaction within timeout"))
		}
	}

	txres, err := rpcClient.GetTransaction(ctx, txsig, &rpc.GetTransactionOpts{
		Commitment: commitment,
	})
	require.NoError(t, err)
	return txres
}

var (
	AddressLookupTableProgram = solana.MustPublicKeyFromBase58("AddressLookupTab1e1111111111111111111111111")
)

const (
	InstructionCreateLookupTable uint32 = iota
	InstructionFreezeLookupTable
	InstructionExtendLookupTable
	InstructionDeactiveLookupTable
	InstructionCloseLookupTable
)

func NewCreateLookupTableInstruction(
	authority, funder solana.PublicKey,
	slot uint64,
) (solana.PublicKey, solana.Instruction, error) {
	// https://github.com/solana-labs/solana-web3.js/blob/c1c98715b0c7900ce37c59bffd2056fa0037213d/src/programs/address-lookup-table/index.ts#L274
	slotLE := make([]byte, 8)
	binary.LittleEndian.PutUint64(slotLE, slot)
	account, bumpSeed, err := solana.FindProgramAddress([][]byte{authority.Bytes(), slotLE}, AddressLookupTableProgram)
	if err != nil {
		return solana.PublicKey{}, nil, err
	}

	data := binary.LittleEndian.AppendUint32([]byte{}, InstructionCreateLookupTable)
	data = binary.LittleEndian.AppendUint64(data, slot)
	data = append(data, bumpSeed)
	return account, solana.NewInstruction(
		AddressLookupTableProgram,
		solana.AccountMetaSlice{
			solana.Meta(account).WRITE(),
			solana.Meta(authority).SIGNER(),
			solana.Meta(funder).SIGNER().WRITE(),
			solana.Meta(solana.SystemProgramID),
		},
		data,
	), nil
}

func NewExtendLookupTableInstruction(
	table, authority, funder solana.PublicKey,
	accounts []solana.PublicKey,
) solana.Instruction {
	// https://github.com/solana-labs/solana-web3.js/blob/c1c98715b0c7900ce37c59bffd2056fa0037213d/src/programs/address-lookup-table/index.ts#L113

	data := binary.LittleEndian.AppendUint32([]byte{}, InstructionExtendLookupTable)
	data = binary.LittleEndian.AppendUint64(data, uint64(len(accounts))) // note: this is usually u32 + 8 byte buffer
	for _, a := range accounts {
		data = append(data, a.Bytes()...)
	}

	return solana.NewInstruction(
		AddressLookupTableProgram,
		solana.AccountMetaSlice{
			solana.Meta(table).WRITE(),
			solana.Meta(authority).SIGNER(),
			solana.Meta(funder).SIGNER().WRITE(),
			solana.Meta(solana.SystemProgramID),
		},
		data,
	)
}

func FundAccounts(t *testing.T, accounts []solana.PrivateKey, solanaGoClient *rpc.Client) {
	ctx := tests.Context(t)
	sigs := []solana.Signature{}
	for _, v := range accounts {
		sig, err := solanaGoClient.RequestAirdrop(ctx, v.PublicKey(), 1000*solana.LAMPORTS_PER_SOL, rpc.CommitmentFinalized)
		require.NoError(t, err)
		sigs = append(sigs, sig)
	}

	// wait for confirmation so later transactions don't fail
	remaining := len(sigs)
	count := 0
	for remaining > 0 {
		count++
		statusRes, sigErr := solanaGoClient.GetSignatureStatuses(ctx, true, sigs...)
		require.NoError(t, sigErr)
		require.NotNil(t, statusRes)
		require.NotNil(t, statusRes.Value)

		unconfirmedTxCount := 0
		for _, res := range statusRes.Value {
			if res == nil || res.ConfirmationStatus == rpc.ConfirmationStatusProcessed || res.ConfirmationStatus == rpc.ConfirmationStatusConfirmed {
				unconfirmedTxCount++
			}
		}
		remaining = unconfirmedTxCount

		time.Sleep(500 * time.Millisecond)
		if count > 60 {
			require.NoError(t, fmt.Errorf("unable to find transaction within timeout"))
		}
	}
}

func SetupTestValidatorWithAnchorPrograms(t *testing.T, upgradeAuthority string, programs []string) (string, string) {
	anchorData := struct {
		Programs struct {
			Localnet map[string]string
		}
	}{}

	// upload programs to validator
	anchorBytes, err := os.ReadFile(PathToAnchorConfig)
	require.NoError(t, err)
	require.NoError(t, toml.Unmarshal(anchorBytes, &anchorData))

	flags := []string{"--warp-slot", "42"}
	for i := range programs {
		k := programs[i]
		v := anchorData.Programs.Localnet[k]
		k = strings.Replace(k, "-", "_", -1)
		flags = append(flags, "--upgradeable-program", v, filepath.Join(ContractsDir, k+".so"), upgradeAuthority)
	}
	rpcURL, wsURL := client.SetupLocalSolNodeWithFlags(t, flags...)
	return rpcURL, wsURL
}

func ConvertAnyToPDASeed(value any) []byte {
	if byteArray, ok := value.([]byte); ok {
		return byteArray
	} else if str, ok := value.(string); ok {
		return []byte(str)
	} else if address, ok := value.(solana.PublicKey); ok {
		return address.Bytes()
	} else if num, ok := value.(uint64); ok {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, num)
		return buf
	} else {
		return nil
	}
}

// GetValuesAtLocation parses through nested types and arrays to find all locations of values
func GetValuesAtLocation(args any, location string) ([][]byte, error) {
	var vals [][]byte
	path := strings.Split(location, ".")

	addressList, err := traversePath(args, path)
	if err != nil {
		return nil, err
	}
	for _, value := range addressList {
		byteArray := ConvertAnyToPDASeed(value)
		if byteArray != nil {
			vals = append(vals, byteArray)
		} else {
			return nil, fmt.Errorf("invalid value format at path: %s", location)
		}
	}

	return vals, nil
}

// traversePath recursively traverses the given structure based on the provided path.
func traversePath(data any, path []string) ([]any, error) {
	if len(path) == 0 {
		return []any{data}, nil
	}

	var result []any

	val := reflect.ValueOf(data)

	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	switch val.Kind() {
	case reflect.Struct:
		field := val.FieldByName(path[0])
		if !field.IsValid() {
			return nil, errors.New("field not found: " + path[0])
		}
		return traversePath(field.Interface(), path[1:])

	case reflect.Slice, reflect.Array:
		for i := 0; i < val.Len(); i++ {
			element := val.Index(i).Interface()
			elements, err := traversePath(element, path)
			if err == nil {
				result = append(result, elements...)
			}
		}
		if len(result) > 0 {
			return result, nil
		}
		return nil, errors.New("no matching field found in array")

	case reflect.Map:
		key := reflect.ValueOf(path[0])
		value := val.MapIndex(key)
		if !value.IsValid() {
			return nil, errors.New("key not found: " + path[0])
		}
		return traversePath(value.Interface(), path[1:])
	default:
		if len(path) == 1 && val.Kind() == reflect.Slice && val.Type().Elem().Kind() == reflect.Uint8 {
			return []any{val.Interface()}, nil
		}
		return nil, errors.New("unexpected type encountered at path: " + path[0])
	}
}
