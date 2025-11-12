package keystone_forwarder_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/common"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/fees"
	"github.com/stretchr/testify/require"

	ctf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	cldf_solana_provider "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana/provider"

	receiver_program "github.com/smartcontractkit/chainlink-solana/contracts/generated/dummy_receiver"
	"github.com/smartcontractkit/chainlink-solana/contracts/generated/keystone_forwarder"
	soltesting "github.com/smartcontractkit/chainlink-solana/pkg/solana/testing"
)

var (
	// Instead of a relative path, use runtime.Caller or go-bindata
	ProgramsPath = getProgramsPath()
)

func getProgramsPath() string {
	// Get the directory of the current file (environment.go)
	_, currentFile, _, _ := runtime.Caller(0)
	// Go up to the root of the deployment package
	rootDir := filepath.Dir(filepath.Dir(currentFile))
	// Construct the absolute path
	return filepath.Join(rootDir, "target", "deploy")
}

var SolanaProgramIDs = map[string]string{
	"keystone_forwarder": "whV7Q5pi17hPPyaPksToDw1nMx6Lh8qmNWKFaLRQ4wz",
	"dummy_receiver":     "5z38tFCAmcPJb1DXUHSoKQhR8qQ8o9aNZ8rZFWe6gH4L",
}

type ConfigSetEvent struct {
	Discriminator [8]byte
	State         solana.PublicKey
	OraclesConfig solana.PublicKey
	DonId         uint32
	ConfigVersion uint32
	F             uint8
	Signers       [][20]uint8
}

type ReportProcessedEvent struct {
	Discriminator  [8]byte
	State          solana.PublicKey
	Receiver       solana.PublicKey
	TransmissionId [32]byte
	Result         bool
}

type Signer struct {
	privKeys  *ecdsa.PrivateKey
	addresses [20]uint8
}

func TestKeystoneForwarder(t *testing.T) {
	const F = uint8(5)
	const workflowExecutionId uint64 = 20
	var reportId uint16 = 11
	const donId uint32 = 7
	const configVersion uint32 = 3
	var solanaClient *rpc.Client
	var deployerKey solana.PrivateKey
	var statepk solana.PrivateKey
	var state solana.PublicKey
	var proposedOwner solana.PrivateKey
	var systemProgram solana.PublicKey
	var forwarderState keystone_forwarder.ForwarderState
	var oraclesConfigData keystone_forwarder.OraclesConfig
	var configSetEvent ConfigSetEvent
	var defaultSigners []Signer
	var forwarderAuthorityStorage solana.PublicKey
	var signers []Signer
	provider := cldf_solana_provider.NewCTFChainProvider(t, 16423721717087811551,
		cldf_solana_provider.CTFChainProviderConfig{
			DeployerKeyGen:               cldf_solana_provider.PrivateKeyRandom(),
			ProgramsPath:                 ProgramsPath,
			ProgramIDs:                   SolanaProgramIDs,
			WaitDelayAfterContainerStart: 5 * time.Second, // we have slot errors that force retries if the chain is not given enough time to boot
		},
	)
	solNode, err := provider.Initialize(t.Context())
	require.NoError(t, err)
	solanaChain := solNode.(ctf_solana.Chain)
	receiver_program_id := solana.MustPublicKeyFromBase58(SolanaProgramIDs["dummy_receiver"])
	t.Run("Setup", func(t *testing.T) {
		// solanaClient = rpc.New("http://localhost:8899")
		// deployerKey, err = solana.NewRandomPrivateKey()
		// require.NoError(t, err)
		solanaClient = rpc.New(solanaChain.URL)
		deployerKey = *solanaChain.DeployerKey
		statepk, err = solana.NewRandomPrivateKey()
		require.NoError(t, err)
		state = statepk.PublicKey()
		proposedOwner, err = solana.NewRandomPrivateKey()
		require.NoError(t, err)
		systemProgram = solana.SystemProgramID
		keystone_forwarder.SetProgramID(solana.MustPublicKeyFromBase58(SolanaProgramIDs["keystone_forwarder"]))
		defaultSigners = generateSigners(t, 16)
	})

	t.Run("Initialize Forwarder", func(t *testing.T) {
		ix, err := keystone_forwarder.NewInitializeInstruction(state, deployerKey.PublicKey(), systemProgram).ValidateAndBuild()
		require.NoError(t, err)
		soltesting.FundTestAccounts(t, []solana.PublicKey{statepk.PublicKey(), deployerKey.PublicKey(), proposedOwner.PublicKey()}, solanaChain.URL)
		res, err := common.SendAndConfirm(
			context.Background(),
			solanaClient, []solana.Instruction{ix}, deployerKey, rpc.CommitmentConfirmed, common.AddSigners(statepk))
		require.NoError(t, err)
		var forwarderState keystone_forwarder.ForwarderState
		err = common.GetAccountDataBorshInto(context.Background(), solanaClient, state, rpc.CommitmentConfirmed, &forwarderState)
		require.NoError(t, err)
		require.Equal(t, forwarderState.Version, uint8(1))
		require.Equal(t, forwarderState.Owner, deployerKey.PublicKey())
		require.Equal(t, forwarderState.ProposedOwner, solana.PublicKey{})

		type ForwarderInitializeEvent struct {
			Discriminator [8]byte
			State         solana.PublicKey
			Owner         solana.PublicKey
		}
		var forwarderInitializeEvent ForwarderInitializeEvent
		err = common.ParseEvent(res.Meta.LogMessages, "ForwarderInitialize", &forwarderInitializeEvent)
		require.NoError(t, err)
	})

	t.Run("Transfer Ownership", func(t *testing.T) {
		transferIx, err := keystone_forwarder.NewTransferOwnershipInstruction(proposedOwner.PublicKey(), state, deployerKey.PublicKey()).ValidateAndBuild()
		require.NoError(t, err)
		_, err = common.SendAndConfirm(
			context.Background(),
			solanaClient, []solana.Instruction{transferIx}, deployerKey, rpc.CommitmentConfirmed)
		require.NoError(t, err)
		err = common.GetAccountDataBorshInto(context.Background(), solanaClient, state, rpc.CommitmentConfirmed, &forwarderState)
		require.NoError(t, err)
		require.Equal(t, forwarderState.Owner, deployerKey.PublicKey())
		require.Equal(t, forwarderState.ProposedOwner, proposedOwner.PublicKey())
	})

	t.Run("Accept Ownership", func(t *testing.T) {
		acceptIx, err := keystone_forwarder.NewAcceptOwnershipInstruction(state, proposedOwner.PublicKey()).ValidateAndBuild()
		require.NoError(t, err)
		_, err = common.SendAndConfirm(
			context.Background(),
			solanaClient, []solana.Instruction{acceptIx}, proposedOwner, rpc.CommitmentConfirmed)
		require.NoError(t, err)
		err = common.GetAccountDataBorshInto(context.Background(), solanaClient, state, rpc.CommitmentConfirmed, &forwarderState)
		require.NoError(t, err)
		require.Equal(t, forwarderState.Owner, proposedOwner.PublicKey())
		require.Equal(t, forwarderState.ProposedOwner, solana.PublicKey{})
	})

	t.Run("Transfer Ownership Back", func(t *testing.T) {
		transferBackIx, err := keystone_forwarder.NewTransferOwnershipInstruction(deployerKey.PublicKey(), state, proposedOwner.PublicKey()).ValidateAndBuild()
		require.NoError(t, err)
		acceptBackIx, err := keystone_forwarder.NewAcceptOwnershipInstruction(state, deployerKey.PublicKey()).ValidateAndBuild()
		require.NoError(t, err)
		_, err = common.SendAndConfirm(
			context.Background(),
			solanaClient, []solana.Instruction{transferBackIx, acceptBackIx}, proposedOwner, rpc.CommitmentConfirmed, common.AddSigners(deployerKey))
		require.NoError(t, err)
		err = common.GetAccountDataBorshInto(context.Background(), solanaClient, state, rpc.CommitmentConfirmed, &forwarderState)
		require.NoError(t, err)
		require.Equal(t, forwarderState.Owner, deployerKey.PublicKey())
		require.Equal(t, forwarderState.ProposedOwner, solana.PublicKey{})
	})

	t.Run("Initialize Oracles Config", func(t *testing.T) {
		f := uint8(1)
		initialEthAddresses := make([][20]uint8, 4)
		for i := 0; i < 4; i++ {
			initialEthAddresses[i] = defaultSigners[i].addresses
		}
		oraclesConfigAddress := getOraclesConfigAddress(t, state, donId, configVersion)
		initOraclesConfigIx, err := keystone_forwarder.NewInitOraclesConfigInstruction(
			donId, configVersion, f, initialEthAddresses, state, oraclesConfigAddress,
			deployerKey.PublicKey(), systemProgram).ValidateAndBuild()
		require.NoError(t, err)
		res, err := common.SendAndConfirm(
			context.Background(),
			solanaClient, []solana.Instruction{initOraclesConfigIx}, deployerKey, rpc.CommitmentConfirmed)
		require.NoError(t, err)
		err = common.GetAccountDataBorshInto(context.Background(), solanaClient, oraclesConfigAddress, rpc.CommitmentConfirmed, &oraclesConfigData)
		require.NoError(t, err)
		require.Equal(t, oraclesConfigData.ConfigId, getConfigId(donId, configVersion))
		require.Equal(t, oraclesConfigData.F, f)

		err = common.ParseEvent(res.Meta.LogMessages, "ConfigSet", &configSetEvent)
		require.NoError(t, err)
		require.Equal(t, configSetEvent.State, state)
		require.Equal(t, configSetEvent.OraclesConfig, oraclesConfigAddress)
		require.Equal(t, configSetEvent.DonId, donId)
		require.Equal(t, configSetEvent.ConfigVersion, configVersion)
		require.Equal(t, configSetEvent.F, f)
		require.Equal(t, configSetEvent.Signers, initialEthAddresses)
	})

	t.Run("Update Oracles Config", func(t *testing.T) {

		oraclesConfigAddress := getOraclesConfigAddress(t, state, donId, configVersion)
		allEthAddresses := make([][20]uint8, len(defaultSigners))
		for i := 0; i < len(defaultSigners); i++ {
			allEthAddresses[i] = defaultSigners[i].addresses
		}
		updateOraclesConfigIx, err := keystone_forwarder.NewUpdateOraclesConfigInstruction(
			donId, configVersion, F, allEthAddresses, state, oraclesConfigAddress,
			deployerKey.PublicKey()).ValidateAndBuild()
		require.NoError(t, err)
		res, err := common.SendAndConfirm(
			context.Background(),
			solanaClient, []solana.Instruction{updateOraclesConfigIx}, deployerKey, rpc.CommitmentConfirmed)
		require.NoError(t, err)
		err = common.GetAccountDataBorshInto(context.Background(), solanaClient, oraclesConfigAddress, rpc.CommitmentConfirmed, &oraclesConfigData)
		require.NoError(t, err)
		require.Equal(t, oraclesConfigData.F, F)
		err = common.ParseEvent(res.Meta.LogMessages, "ConfigSet", &configSetEvent)
		require.NoError(t, err)
		require.Equal(t, configSetEvent.State, state)
		require.Equal(t, configSetEvent.OraclesConfig, oraclesConfigAddress)
		require.Equal(t, configSetEvent.DonId, donId)
		require.Equal(t, configSetEvent.ConfigVersion, configVersion)
		require.Equal(t, configSetEvent.F, F)
		require.Equal(t, len(configSetEvent.Signers), len(allEthAddresses))
	})

	t.Run("Report", func(t *testing.T) {
		require.NoError(t, err)
		receiver_program.SetProgramID(receiver_program_id)
		reportState, err := solana.NewRandomPrivateKey()
		require.NoError(t, err)
		forwarderAuthorityStorage, _, err = solana.FindProgramAddress(
			[][]byte{[]byte("forwarder"), state.Bytes(), receiver_program_id.Bytes()},
			keystone_forwarder.ProgramID,
		)
		require.NoError(t, err)
		// receiver program initialize
		receiverInit, err := receiver_program.NewInitializeInstruction(
			reportState.PublicKey(), deployerKey.PublicKey(), forwarderAuthorityStorage, systemProgram).ValidateAndBuild()
		require.NoError(t, err)
		_, err = common.SendAndConfirm(
			context.Background(),
			solanaClient, []solana.Instruction{receiverInit}, deployerKey, rpc.CommitmentConfirmed, common.AddSigners(reportState))
		require.NoError(t, err)

		accountHash := GenerateAccountHash(state, forwarderAuthorityStorage, reportState.PublicKey())

		transmissionId := GetTransmissionId(workflowExecutionId, reportId, receiver_program_id)
		executionStateStorage, _, err := solana.FindProgramAddress(
			[][]byte{[]byte("execution_state"), state.Bytes(), transmissionId},
			keystone_forwarder.ProgramID,
		)
		require.NoError(t, err)

		signers = make([]Signer, F+1)
		for i := 0; i < int(F+1); i++ {
			signers[i] = defaultSigners[i]
		}
		var lenSignatureBytes [1]byte
		lenSignatureBytes[0] = byte(len(signers))

		payload := []byte{255}
		forwarderReportBytes, err := EncodeForwarderReport(accountHash, payload)
		rawReportBytes := make([]byte, 109+len(forwarderReportBytes))
		copy(rawReportBytes[:109], BuildRawReportBytes(reportId))
		copy(rawReportBytes[109:], forwarderReportBytes)

		msgHash32, reportContext96 := BuildMessageHash(rawReportBytes)
		require.NoError(t, err)
		require.Equal(t, len(msgHash32), 32)
		require.Equal(t, len(reportContext96), 96)

		dataBytes, err := PackDataWithSignatures(rawReportBytes, msgHash32, reportContext96, signers)
		require.NoError(t, err)

		fwdOnReportIx := keystone_forwarder.NewReportInstruction(
			dataBytes,
			state,
			getOraclesConfigAddress(t, state, donId, configVersion),
			deployerKey.PublicKey(),
			forwarderAuthorityStorage,
			executionStateStorage,
			receiver_program_id,
			systemProgram,
		)
		fwdOnReportIx.Append(&solana.AccountMeta{
			PublicKey:  reportState.PublicKey(),
			IsWritable: true,
			IsSigner:   false,
		})
		fwdOnReportIx2, err := fwdOnReportIx.ValidateAndBuild()
		require.NoError(t, err)

		res, err := common.SendAndConfirm(
			context.Background(),
			solanaClient, []solana.Instruction{fwdOnReportIx2}, deployerKey, rpc.CommitmentConfirmed, common.AddComputeUnitLimit(fees.ComputeUnitLimit(1_400_000)))
		require.NoError(t, err)

		var reportProcessedEvent ReportProcessedEvent
		err = common.ParseEvent(res.Meta.LogMessages, "ReportProcessed", &reportProcessedEvent)
		require.NoError(t, err)
		require.Equal(t, state, reportProcessedEvent.State)
		require.Equal(t, receiver_program_id, reportProcessedEvent.Receiver)
		require.Equal(t, [32]byte(transmissionId), reportProcessedEvent.TransmissionId)
		require.Equal(t, true, reportProcessedEvent.Result)

		var executionState keystone_forwarder.ExecutionState
		err = common.GetAccountDataBorshInto(context.Background(), solanaClient, executionStateStorage, rpc.CommitmentConfirmed, &executionState)
		require.NoError(t, err)
		require.Equal(t, true, executionState.Success)
		require.Equal(t, [32]byte(transmissionId), executionState.TransmissionId)
		require.Equal(t, deployerKey.PublicKey(), executionState.Transmitter)

		// check on the reciever end
		var latestReportAccount receiver_program.LatestReport
		err = common.GetAccountDataBorshInto(context.Background(), solanaClient, reportState.PublicKey(), rpc.CommitmentConfirmed, &latestReportAccount)
		require.NoError(t, err)
		require.Equal(t, payload, latestReportAccount.Report)
		require.Equal(t, forwarderAuthorityStorage, latestReportAccount.ForwarderAuthority)
		require.Equal(t, rawReportBytes[45:109], latestReportAccount.Metadata)

		// send the same report again, should fail with ExecutionAlreadySucceded error
		res, err = common.SendAndFailWith(
			context.Background(),
			solanaClient, []solana.Instruction{fwdOnReportIx2}, deployerKey, rpc.CommitmentConfirmed, []string{"Execution already succeded"}, common.AddComputeUnitLimit(fees.ComputeUnitLimit(1_400_000)))
		require.NoError(t, err)
	})

	t.Run("Report Failure", func(t *testing.T) {

		require.NoError(t, err)
		failedReportState, err := solana.NewRandomPrivateKey()
		require.NoError(t, err)

		accountHash := GenerateAccountHash(state, forwarderAuthorityStorage, failedReportState.PublicKey())
		failReportId := reportId + 1
		transmissionId := GetTransmissionId(workflowExecutionId, failReportId, receiver_program_id)
		executionStateStorage, _, err := solana.FindProgramAddress(
			[][]byte{[]byte("execution_state"), state.Bytes(), transmissionId},
			keystone_forwarder.ProgramID,
		)
		require.NoError(t, err)

		var lenSignatureBytes [1]byte
		lenSignatureBytes[0] = byte(len(signers))

		payload := []byte{255}
		forwarderReportBytes, err := EncodeForwarderReport(accountHash, payload)
		rawReportBytes := make([]byte, 109+len(forwarderReportBytes))
		copy(rawReportBytes[:109], BuildRawReportBytes(failReportId))
		copy(rawReportBytes[109:], forwarderReportBytes)

		msgHash32, reportContext96 := BuildMessageHash(rawReportBytes)
		require.NoError(t, err)
		require.Equal(t, len(msgHash32), 32)
		require.Equal(t, len(reportContext96), 96)

		dataBytes, err := PackDataWithSignatures(rawReportBytes, msgHash32, reportContext96, signers)
		require.NoError(t, err)

		fwdOnReportFailureIx := keystone_forwarder.NewReportFailureInstruction(
			dataBytes,
			state,
			getOraclesConfigAddress(t, state, donId, configVersion),
			deployerKey.PublicKey(),
			forwarderAuthorityStorage,
			executionStateStorage,
			receiver_program_id,
			systemProgram,
		)
		fwdOnReportFailureIx.Append(&solana.AccountMeta{
			PublicKey:  failedReportState.PublicKey(),
			IsWritable: true,
			IsSigner:   false,
		})
		fwdOnReportFailureIx2, err := fwdOnReportFailureIx.ValidateAndBuild()
		require.NoError(t, err)

		res, err := common.SendAndConfirm(
			context.Background(),
			solanaClient, []solana.Instruction{fwdOnReportFailureIx2}, deployerKey, rpc.CommitmentConfirmed, common.AddComputeUnitLimit(fees.ComputeUnitLimit(1_400_000)))
		require.NoError(t, err)

		var reportProcessedEvent ReportProcessedEvent
		err = common.ParseEvent(res.Meta.LogMessages, "ReportProcessed", &reportProcessedEvent)
		require.NoError(t, err)
		require.Equal(t, state, reportProcessedEvent.State)
		require.Equal(t, receiver_program_id, reportProcessedEvent.Receiver)
		require.Equal(t, [32]byte(transmissionId), reportProcessedEvent.TransmissionId)
		require.Equal(t, false, reportProcessedEvent.Result)

		var executionState keystone_forwarder.ExecutionState
		err = common.GetAccountDataBorshInto(context.Background(), solanaClient, executionStateStorage, rpc.CommitmentConfirmed, &executionState)
		require.NoError(t, err)
		require.Equal(t, false, executionState.Success)
		require.Equal(t, true, executionState.Failure)
		require.Equal(t, [32]byte(transmissionId), executionState.TransmissionId)
		require.Equal(t, deployerKey.PublicKey(), executionState.Transmitter)
	})
}

func PackDataWithSignatures(rawReportBytes, msgHash32, reportContext96 []byte, signers []Signer) ([]byte, error) {
	// 1) Sign with each signer; pack as [64-byte sig || 1-byte recid] per signer
	var sigBlob bytes.Buffer
	for i, s := range signers {
		if s.privKeys == nil {
			return nil, fmt.Errorf("signer %d has nil private key", i)
		}
		sig65, err := crypto.Sign(msgHash32, s.privKeys) // 65 bytes: R(32)||S(32)||V(1)
		if err != nil {
			return nil, fmt.Errorf("signer %d: %w", i, err)
		}
		// TS: returns {signature: 64 bytes, recovery: 1 byte}
		sigBlob.Write(sig65[:64])    // signature (R||S)
		sigBlob.WriteByte(sig65[64]) // recovery id (V)
	}

	// 2) Prefix with len(signers) as a single byte (u8)
	lenByte := []byte{byte(len(signers) & 0xff)}

	// 3) Final data: len(1) | signatures(N*65) | raw_report | report_context(96)
	final := bytes.Join([][]byte{
		lenByte,
		sigBlob.Bytes(),
		rawReportBytes,
		reportContext96,
	}, nil)

	return final, nil
}

func GenerateAccountHash(state solana.PublicKey, forwarderAuthority solana.PublicKey, reportState solana.PublicKey) []byte {
	hasher := sha256.New()
	hasher.Write(state.Bytes())
	hasher.Write(forwarderAuthority.Bytes())
	hasher.Write(reportState.Bytes())
	return hasher.Sum(nil)
}

func GetTransmissionId(workflowExecutionId uint64, reportId uint16, receiverProgramId solana.PublicKey) []byte {
	workflowExecutionIdBytes := make([]byte, 32)
	workflowExecutionIdBytes[31] = byte(workflowExecutionId)
	reportIdBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(reportIdBytes, reportId)
	hasher := sha256.New()
	hasher.Write(receiverProgramId.Bytes())
	hasher.Write(workflowExecutionIdBytes)
	hasher.Write(reportIdBytes)
	return hasher.Sum(nil)
}

func BuildMessageHash(rawReportBytes []byte) (msgHash32 []byte, reportContext96 []byte) {
	reportContext96 = make([]byte, 96) // zeroed, just like TS
	rawLen1 := []byte{byte(len(rawReportBytes) & 0xff)}
	h := sha256.New()
	h.Write(rawLen1)
	h.Write(rawReportBytes)
	h.Write(reportContext96)
	return h.Sum(nil), reportContext96 // 32-byte hash
}

func generateSigners(t *testing.T, n int) []Signer {
	signers := make([]Signer, n)
	for i := 0; i < n; i++ {
		privKey, err := crypto.GenerateKey()
		require.NoError(t, err)
		// privKeys[i] = privKey
		address := crypto.PubkeyToAddress(privKey.PublicKey).Bytes()
		uint8Address := [20]uint8{}
		copy(uint8Address[:], address)
		// addresses[i] = uint8Address
		signers[i] = Signer{
			privKeys:  privKey,
			addresses: uint8Address,
		}
	}
	sort.Slice(signers, func(i, j int) bool {
		return bytes.Compare(signers[i].addresses[:], signers[j].addresses[:]) < 0
	})
	return signers
}

// -----------------------------------------------------------------------------
//  1. ForwarderReport encode: account_hash(32) | payload_len(u32 LE) | payload
//     (TS code allocs 32 + 4 + len(payload); we mirror that exactly.)
//
// -----------------------------------------------------------------------------
func EncodeForwarderReport(accountHash32 []byte, payload []byte) ([]byte, error) {
	if len(accountHash32) != 32 {
		return nil, fmt.Errorf("accountHash must be 32 bytes; got %d", len(accountHash32))
	}
	out := make([]byte, 32+4+len(payload))
	copy(out[:32], accountHash32)
	binary.LittleEndian.PutUint32(out[32:36], uint32(len(payload)))
	copy(out[36:], payload)
	return out, nil
}

// version                offset   0, size  1
// workflow_execution_id  offset   1, size 32
// timestamp              offset  33, size  4
// don_id                 offset  37, size  4
// don_config_version     offset  41, size  4
// workflow_cid           offset  45, size 32
// workflow_name          offset  77, size 10
// workflow_owner         offset  87, size 20
// report_id              offset 107, size  2
func BuildRawReportBytes(reportId uint16) []byte {
	raw := make([]byte, 109)
	raw[0] = 1
	raw[32] = 20 // last byte of a 32-byte lane (as in TS)
	raw[36] = 5
	raw[40] = 7
	raw[44] = 3
	raw[76] = 2
	raw[86] = 10
	raw[106] = 11
	raw[108] = byte(reportId)

	return raw
}

func getConfigId(donId uint32, configVersion uint32) uint64 {
	return (uint64(donId) << 32) | uint64(configVersion)
}

func getOraclesConfigAddress(t *testing.T, state solana.PublicKey, donId uint32, configVersion uint32) solana.PublicKey {
	configId := getConfigId(donId, configVersion)
	var cfgIDBE [8]byte
	binary.BigEndian.PutUint64(cfgIDBE[:], configId)
	oraclesConfigAddress, _, _ := solana.FindProgramAddress(
		[][]byte{[]byte("config"), state.Bytes(), cfgIDBE[:]},
		keystone_forwarder.ProgramID,
	)
	return oraclesConfigAddress
}
