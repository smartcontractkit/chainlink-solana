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

const (
	workflowExecutionId uint64 = 20
	reportId            uint16 = 11
	donId               uint32 = 7
	configVersion       uint32 = 3
	F                          = uint8(5)
)

func TestKeystoneForwarder(t *testing.T) {
	var solanaClient *rpc.Client
	var deployerKey solana.PrivateKey

	// forwarder state
	var forwarderStateKey solana.PrivateKey
	var forwarderStateAddress solana.PublicKey
	var forwarderStateData keystone_forwarder.ForwarderState

	// proposed owner for ownership transfer
	var proposedOwner solana.PrivateKey

	// oracles config data for the forwarder
	var oraclesConfigData keystone_forwarder.OraclesConfig

	// events
	var configSetEvent ConfigSetEvent
	var reportProcessedEvent ReportProcessedEvent

	// forwarder authority storage for the receiver program
	var forwarderAuthorityStorage solana.PublicKey

	// signers for the report
	var defaultSigners []Signer

	provider := cldf_solana_provider.NewCTFChainProvider(t, 16423721717087811551,
		cldf_solana_provider.CTFChainProviderConfig{
			DeployerKeyGen:               cldf_solana_provider.PrivateKeyRandom(),
			ProgramsPath:                 ProgramsPath,
			ProgramIDs:                   SolanaProgramIDs,
			WaitDelayAfterContainerStart: 5 * time.Second, // we have slot errors that force retries if the chain is not given enough time to boot
		},
	)
	solanaNode, err := provider.Initialize(t.Context())
	require.NoError(t, err)
	solanaChain := solanaNode.(ctf_solana.Chain)
	receiver_program_id := solana.MustPublicKeyFromBase58(SolanaProgramIDs["dummy_receiver"])

	t.Run("Setup", func(t *testing.T) {
		// solanaClient = rpc.New("http://localhost:8899")
		// deployerKey, err = solana.NewRandomPrivateKey()
		// require.NoError(t, err)
		solanaClient = rpc.New(solanaChain.URL)
		deployerKey = *solanaChain.DeployerKey
		forwarderStateKey, err = solana.NewRandomPrivateKey()
		require.NoError(t, err)
		forwarderStateAddress = forwarderStateKey.PublicKey()
		proposedOwner, err = solana.NewRandomPrivateKey()
		require.NoError(t, err)
		keystone_forwarder.SetProgramID(solana.MustPublicKeyFromBase58(SolanaProgramIDs["keystone_forwarder"]))
		receiver_program.SetProgramID(receiver_program_id)
		defaultSigners = generateSigners(t, 16)
		forwarderAuthorityStorage, _, err = solana.FindProgramAddress(
			[][]byte{[]byte("forwarder"), forwarderStateAddress.Bytes(), receiver_program_id.Bytes()},
			keystone_forwarder.ProgramID,
		)
		require.NoError(t, err)
	})

	t.Run("Initialize Forwarder", func(t *testing.T) {
		ix, err := keystone_forwarder.NewInitializeInstruction(forwarderStateAddress, deployerKey.PublicKey(), solana.SystemProgramID).ValidateAndBuild()
		require.NoError(t, err)
		soltesting.FundTestAccounts(t, []solana.PublicKey{forwarderStateKey.PublicKey(), deployerKey.PublicKey(), proposedOwner.PublicKey()}, solanaChain.URL)
		res, err := common.SendAndConfirm(
			context.Background(),
			solanaClient, []solana.Instruction{ix}, deployerKey, rpc.CommitmentConfirmed, common.AddSigners(forwarderStateKey))
		require.NoError(t, err)
		err = common.GetAccountDataBorshInto(context.Background(), solanaClient, forwarderStateAddress, rpc.CommitmentConfirmed, &forwarderStateData)
		require.NoError(t, err)
		require.Equal(t, forwarderStateData.Version, uint8(1))
		require.Equal(t, forwarderStateData.Owner, deployerKey.PublicKey())
		require.Equal(t, forwarderStateData.ProposedOwner, solana.PublicKey{})

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
		transferIx, err := keystone_forwarder.NewTransferOwnershipInstruction(proposedOwner.PublicKey(), forwarderStateAddress, deployerKey.PublicKey()).ValidateAndBuild()
		require.NoError(t, err)
		_, err = common.SendAndConfirm(
			context.Background(),
			solanaClient, []solana.Instruction{transferIx}, deployerKey, rpc.CommitmentConfirmed)
		require.NoError(t, err)
		err = common.GetAccountDataBorshInto(context.Background(), solanaClient, forwarderStateAddress, rpc.CommitmentConfirmed, &forwarderStateData)
		require.NoError(t, err)
		require.Equal(t, forwarderStateData.Owner, deployerKey.PublicKey())
		require.Equal(t, forwarderStateData.ProposedOwner, proposedOwner.PublicKey())
	})

	t.Run("Accept Ownership", func(t *testing.T) {
		acceptIx, err := keystone_forwarder.NewAcceptOwnershipInstruction(forwarderStateAddress, proposedOwner.PublicKey()).ValidateAndBuild()
		require.NoError(t, err)
		_, err = common.SendAndConfirm(
			context.Background(),
			solanaClient, []solana.Instruction{acceptIx}, proposedOwner, rpc.CommitmentConfirmed)
		require.NoError(t, err)
		err = common.GetAccountDataBorshInto(context.Background(), solanaClient, forwarderStateAddress, rpc.CommitmentConfirmed, &forwarderStateData)
		require.NoError(t, err)
		require.Equal(t, forwarderStateData.Owner, proposedOwner.PublicKey())
		require.Equal(t, forwarderStateData.ProposedOwner, solana.PublicKey{})
	})

	t.Run("Transfer Ownership Back", func(t *testing.T) {
		transferBackIx, err := keystone_forwarder.NewTransferOwnershipInstruction(
			deployerKey.PublicKey(),
			forwarderStateAddress,
			proposedOwner.PublicKey(),
		).ValidateAndBuild()
		require.NoError(t, err)
		acceptBackIx, err := keystone_forwarder.NewAcceptOwnershipInstruction(
			forwarderStateAddress,
			deployerKey.PublicKey(),
		).ValidateAndBuild()
		require.NoError(t, err)
		_, err = common.SendAndConfirm(
			context.Background(),
			solanaClient, []solana.Instruction{transferBackIx, acceptBackIx}, proposedOwner, rpc.CommitmentConfirmed, common.AddSigners(deployerKey))
		require.NoError(t, err)
		err = common.GetAccountDataBorshInto(context.Background(), solanaClient, forwarderStateAddress, rpc.CommitmentConfirmed, &forwarderStateData)
		require.NoError(t, err)
		require.Equal(t, forwarderStateData.Owner, deployerKey.PublicKey())
		require.Equal(t, forwarderStateData.ProposedOwner, solana.PublicKey{})
	})

	t.Run("Initialize Oracles Config", func(t *testing.T) {
		f := uint8(1)
		initialEthAddresses := make([][20]uint8, 4)
		for i := 0; i < 4; i++ {
			initialEthAddresses[i] = defaultSigners[i].addresses
		}
		oraclesConfigAddress := getOraclesConfigAddress(t, forwarderStateAddress, donId, configVersion)
		initOraclesConfigIx, err := keystone_forwarder.NewInitOraclesConfigInstruction(
			donId, configVersion, f, initialEthAddresses, forwarderStateAddress, oraclesConfigAddress,
			deployerKey.PublicKey(), solana.SystemProgramID).ValidateAndBuild()
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
		require.Equal(t, configSetEvent.State, forwarderStateAddress)
		require.Equal(t, configSetEvent.OraclesConfig, oraclesConfigAddress)
		require.Equal(t, configSetEvent.DonId, donId)
		require.Equal(t, configSetEvent.ConfigVersion, configVersion)
		require.Equal(t, configSetEvent.F, f)
		require.Equal(t, configSetEvent.Signers, initialEthAddresses)
	})

	t.Run("Update Oracles Config", func(t *testing.T) {
		oraclesConfigAddress := getOraclesConfigAddress(t, forwarderStateAddress, donId, configVersion)
		allEthAddresses := make([][20]uint8, len(defaultSigners))
		for i := 0; i < len(defaultSigners); i++ {
			allEthAddresses[i] = defaultSigners[i].addresses
		}
		updateOraclesConfigIx, err := keystone_forwarder.NewUpdateOraclesConfigInstruction(
			donId, configVersion, F, allEthAddresses, forwarderStateAddress, oraclesConfigAddress,
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
		require.Equal(t, configSetEvent.State, forwarderStateAddress)
		require.Equal(t, configSetEvent.OraclesConfig, oraclesConfigAddress)
		require.Equal(t, configSetEvent.DonId, donId)
		require.Equal(t, configSetEvent.ConfigVersion, configVersion)
		require.Equal(t, configSetEvent.F, F)
		require.Equal(t, len(configSetEvent.Signers), len(allEthAddresses))
	})

	t.Run("Report", func(t *testing.T) {
		reportState, err := solana.NewRandomPrivateKey()
		require.NoError(t, err)
		initializeReceiverProgram(t, reportState, deployerKey, forwarderAuthorityStorage, solanaClient)
		accountHash := generateAccountHash(forwarderStateAddress, forwarderAuthorityStorage, reportState.PublicKey())
		transmissionId := getTransmissionId(workflowExecutionId, reportId, receiver_program_id)
		executionStateStorage, _, err := solana.FindProgramAddress(
			[][]byte{[]byte("execution_state"), forwarderStateAddress.Bytes(), transmissionId},
			keystone_forwarder.ProgramID,
		)
		require.NoError(t, err)

		signers := getFSigners(t, defaultSigners, F)
		payload := []byte{255}
		dataBytes, rawReportBytes := getDataBytes(t, accountHash, payload, reportId, signers)

		fwdOnReportIx := keystone_forwarder.NewReportInstruction(
			dataBytes,
			forwarderStateAddress,
			getOraclesConfigAddress(t, forwarderStateAddress, donId, configVersion),
			deployerKey.PublicKey(),
			forwarderAuthorityStorage,
			executionStateStorage,
			receiver_program_id,
			solana.SystemProgramID,
		)
		fwdOnReportIx.Append(&solana.AccountMeta{
			PublicKey:  reportState.PublicKey(),
			IsWritable: true,
			IsSigner:   false,
		})
		fwdOnReportIxWithRemainingAccounts, err := fwdOnReportIx.ValidateAndBuild()
		require.NoError(t, err)

		res, err := common.SendAndConfirm(
			context.Background(),
			solanaClient,
			[]solana.Instruction{fwdOnReportIxWithRemainingAccounts},
			deployerKey,
			rpc.CommitmentConfirmed,
			common.AddComputeUnitLimit(fees.ComputeUnitLimit(1_400_000)),
		)
		require.NoError(t, err)

		err = common.ParseEvent(res.Meta.LogMessages, "ReportProcessed", &reportProcessedEvent)
		require.NoError(t, err)
		require.Equal(t, forwarderStateAddress, reportProcessedEvent.State)
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
			solanaClient,
			[]solana.Instruction{fwdOnReportIxWithRemainingAccounts},
			deployerKey,
			rpc.CommitmentConfirmed,
			[]string{"Execution already succeded"},
			common.AddComputeUnitLimit(fees.ComputeUnitLimit(1_400_000)),
		)
		require.NoError(t, err)
	})

	t.Run("Report Failure", func(t *testing.T) {
		failedReportState, err := solana.NewRandomPrivateKey()
		require.NoError(t, err)

		initializeReceiverProgram(t, failedReportState, deployerKey, forwarderAuthorityStorage, solanaClient)
		accountHash := generateAccountHash(forwarderStateAddress, forwarderAuthorityStorage, failedReportState.PublicKey())
		failReportId := reportId + 1
		transmissionId := getTransmissionId(workflowExecutionId, failReportId, receiver_program_id)
		executionStateStorage, _, err := solana.FindProgramAddress(
			[][]byte{[]byte("execution_state"), forwarderStateAddress.Bytes(), transmissionId},
			keystone_forwarder.ProgramID,
		)
		require.NoError(t, err)

		signers := getFSigners(t, defaultSigners, F)
		payload := []byte{255}
		dataBytes, _ := getDataBytes(t, accountHash, payload, failReportId, signers)

		fwdOnReportFailureIx := keystone_forwarder.NewReportFailureInstruction(
			dataBytes,
			forwarderStateAddress,
			getOraclesConfigAddress(t, forwarderStateAddress, donId, configVersion),
			deployerKey.PublicKey(),
			forwarderAuthorityStorage,
			executionStateStorage,
			receiver_program_id,
			solana.SystemProgramID,
		)
		fwdOnReportFailureIx.Append(&solana.AccountMeta{
			PublicKey:  failedReportState.PublicKey(),
			IsWritable: true,
			IsSigner:   false,
		})
		fwdOnReportFailureIxWithRemainingAccounts, err := fwdOnReportFailureIx.ValidateAndBuild()
		require.NoError(t, err)

		res, err := common.SendAndConfirm(
			context.Background(),
			solanaClient, []solana.Instruction{fwdOnReportFailureIxWithRemainingAccounts}, deployerKey, rpc.CommitmentConfirmed, common.AddComputeUnitLimit(fees.ComputeUnitLimit(1_400_000)))
		require.NoError(t, err)

		err = common.ParseEvent(res.Meta.LogMessages, "ReportProcessed", &reportProcessedEvent)
		require.NoError(t, err)
		require.Equal(t, forwarderStateAddress, reportProcessedEvent.State)
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

		res, err = common.SendAndFailWith(
			context.Background(),
			solanaClient, []solana.Instruction{fwdOnReportFailureIxWithRemainingAccounts}, deployerKey, rpc.CommitmentConfirmed, []string{"Execution already marked failed"}, common.AddComputeUnitLimit(fees.ComputeUnitLimit(1_400_000)))
		require.NoError(t, err)
	})
}

func packDataWithSignatures(rawReportBytes, msgHash32, reportContext96 []byte, signers []Signer) ([]byte, error) {
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

// hash the accounts together
func generateAccountHash(state solana.PublicKey, forwarderAuthority solana.PublicKey, reportState solana.PublicKey) []byte {
	hasher := sha256.New()
	hasher.Write(state.Bytes())
	hasher.Write(forwarderAuthority.Bytes())
	hasher.Write(reportState.Bytes()) // this would be all of the remaining accounts in onchain code
	return hasher.Sum(nil)
}

// hash the workflow execution id, report id and receiver program id together
func getTransmissionId(workflowExecutionId uint64, reportId uint16, receiverProgramId solana.PublicKey) []byte {
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

// hash the rawrawReportBytes with report length and context
func buildMessageHash(rawReportBytes []byte) (msgHash32 []byte, reportContext96 []byte) {
	reportContext96 = make([]byte, 96) // fixed length on chain
	rawLen := []byte{byte(len(rawReportBytes) & 0xff)}
	h := sha256.New()
	h.Write(rawLen)
	h.Write(rawReportBytes)
	h.Write(reportContext96)
	return h.Sum(nil), reportContext96 // 32-byte hash
}

func generateSigners(t *testing.T, n int) []Signer {
	signers := make([]Signer, n)
	for i := 0; i < n; i++ {
		privKey, err := crypto.GenerateKey()
		require.NoError(t, err)
		address := crypto.PubkeyToAddress(privKey.PublicKey).Bytes()
		uint8Address := [20]uint8{}
		copy(uint8Address[:], address)
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

func getFSigners(t *testing.T, signers []Signer, f uint8) []Signer {
	return signers[:f+1]
}

func initializeReceiverProgram(t *testing.T, reportState solana.PrivateKey, deployerKey solana.PrivateKey, forwarderAuthorityStorage solana.PublicKey, solanaClient *rpc.Client) {
	// receiver program initialize
	receiverInit, err := receiver_program.NewInitializeInstruction(
		reportState.PublicKey(),
		deployerKey.PublicKey(),
		forwarderAuthorityStorage,
		solana.SystemProgramID,
	).ValidateAndBuild()
	require.NoError(t, err)

	_, err = common.SendAndConfirm(
		context.Background(),
		solanaClient,
		[]solana.Instruction{receiverInit},
		deployerKey,
		rpc.CommitmentConfirmed,
		common.AddSigners(reportState),
	)
	require.NoError(t, err)
}

// encode forwarder report
// build metadata
// concatenate
// hash the message
// sign the hashed message
// pack it all together
func getDataBytes(t *testing.T, accountHash []byte, payload []byte, reportId uint16, signers []Signer) ([]byte, []byte) {
	forwarderReportBytes := encodeForwarderReport(accountHash, payload)
	metadataBytes := buildRawReportBytes(reportId)
	// 109 is the length of the metadata
	rawReportBytes := make([]byte, 109+len(forwarderReportBytes))
	// metadata
	copy(rawReportBytes[:109], metadataBytes)
	// forwarder report
	copy(rawReportBytes[109:], forwarderReportBytes)

	msgHash32, reportContext96 := buildMessageHash(rawReportBytes)
	require.Equal(t, len(msgHash32), 32)
	require.Equal(t, len(reportContext96), 96)

	dataBytes, err := packDataWithSignatures(rawReportBytes, msgHash32, reportContext96, signers)
	require.NoError(t, err)
	return dataBytes, rawReportBytes
}

// ForwarderReport encode: account_hash(32) | payload_len(u32 LE) | payload
//
// #[derive(BorshDeserialize)]
//
//	pub struct ForwarderReport {
//		pub account_hash: [u8; 32],
//		pub payload: Vec<u8>,
//	}
func encodeForwarderReport(accountHash32 []byte, payload []byte) []byte {
	out := make([]byte, 32+4+len(payload))
	copy(out[:32], accountHash32)
	binary.LittleEndian.PutUint32(out[32:36], uint32(len(payload)))
	copy(out[36:], payload)
	return out
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
func buildRawReportBytes(reportId uint16) []byte {
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

// TODO:
// add comments (ideally link onchain code or something)
// get this test file polished
// write data feeds cache test
