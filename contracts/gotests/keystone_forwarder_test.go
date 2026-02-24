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
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("failed to get current file path")
	}
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
	privKeys *ecdsa.PrivateKey
	address  [20]uint8
}

type Transmitter struct {
	privKey solana.PrivateKey
	address [32]uint8
}

const (
	workflowExecutionId uint64 = 20
	reportId            uint16 = 11
	donId               uint32 = 7
	configVersion       uint32 = 3
	F                          = uint8(5)
	MetadataLength             = 109
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

	// transmitters (Solana key pairs and addresses)
	var defaultTransmitters []Transmitter

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
		defaultTransmitters = generateTransmitters(t, deployerKey, 16)
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
			t.Context(),
			solanaClient, []solana.Instruction{ix}, deployerKey, rpc.CommitmentConfirmed, common.AddSigners(forwarderStateKey))
		require.NoError(t, err)
		err = common.GetAccountDataBorshInto(t.Context(), solanaClient, forwarderStateAddress, rpc.CommitmentConfirmed, &forwarderStateData)
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
			t.Context(),
			solanaClient, []solana.Instruction{transferIx}, deployerKey, rpc.CommitmentConfirmed)
		require.NoError(t, err)
		err = common.GetAccountDataBorshInto(t.Context(), solanaClient, forwarderStateAddress, rpc.CommitmentConfirmed, &forwarderStateData)
		require.NoError(t, err)
		require.Equal(t, forwarderStateData.Owner, deployerKey.PublicKey())
		require.Equal(t, forwarderStateData.ProposedOwner, proposedOwner.PublicKey())
	})

	t.Run("Accept Ownership", func(t *testing.T) {
		acceptIx, err := keystone_forwarder.NewAcceptOwnershipInstruction(forwarderStateAddress, proposedOwner.PublicKey()).ValidateAndBuild()
		require.NoError(t, err)
		_, err = common.SendAndConfirm(
			t.Context(),
			solanaClient, []solana.Instruction{acceptIx}, proposedOwner, rpc.CommitmentConfirmed)
		require.NoError(t, err)
		err = common.GetAccountDataBorshInto(t.Context(), solanaClient, forwarderStateAddress, rpc.CommitmentConfirmed, &forwarderStateData)
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
			t.Context(),
			solanaClient, []solana.Instruction{transferBackIx, acceptBackIx}, proposedOwner, rpc.CommitmentConfirmed, common.AddSigners(deployerKey))
		require.NoError(t, err)
		err = common.GetAccountDataBorshInto(t.Context(), solanaClient, forwarderStateAddress, rpc.CommitmentConfirmed, &forwarderStateData)
		require.NoError(t, err)
		require.Equal(t, forwarderStateData.Owner, deployerKey.PublicKey())
		require.Equal(t, forwarderStateData.ProposedOwner, solana.PublicKey{})
	})

	t.Run("Initialize Oracles Config", func(t *testing.T) {
		f := uint8(1)
		initialEthAddresses := make([][20]uint8, 4)
		initialTransmitterAddresses := make([][32]uint8, 4)
		for i := 0; i < 4; i++ {
			initialEthAddresses[i] = defaultSigners[i].address
			initialTransmitterAddresses[i] = defaultTransmitters[i].address
		}
		oraclesConfigAddress := getOraclesConfigAddress(t, forwarderStateAddress, donId, configVersion)
		initOraclesConfigIx, err := keystone_forwarder.NewInitOraclesConfigInstruction(
			donId, configVersion, f, initialEthAddresses, forwarderStateAddress, oraclesConfigAddress,
			deployerKey.PublicKey(), solana.SystemProgramID).ValidateAndBuild()
		require.NoError(t, err)
		res, err := common.SendAndConfirm(
			t.Context(),
			solanaClient, []solana.Instruction{initOraclesConfigIx}, deployerKey, rpc.CommitmentConfirmed)
		require.NoError(t, err)
		err = common.GetAccountDataBorshInto(t.Context(), solanaClient, oraclesConfigAddress, rpc.CommitmentConfirmed, &oraclesConfigData)
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
		allTransmitterAddresses := make([][32]uint8, len(defaultTransmitters))
		for i := 0; i < len(defaultSigners); i++ {
			allEthAddresses[i] = defaultSigners[i].address
			allTransmitterAddresses[i] = defaultTransmitters[i].address
		}
		updateOraclesConfigIx, err := keystone_forwarder.NewUpdateOraclesConfigInstruction(
			donId, configVersion, F, allEthAddresses, forwarderStateAddress, oraclesConfigAddress,
			deployerKey.PublicKey()).ValidateAndBuild()
		require.NoError(t, err)
		res, err := common.SendAndConfirm(
			t.Context(),
			solanaClient, []solana.Instruction{updateOraclesConfigIx}, deployerKey, rpc.CommitmentConfirmed)
		require.NoError(t, err)
		err = common.GetAccountDataBorshInto(t.Context(), solanaClient, oraclesConfigAddress, rpc.CommitmentConfirmed, &oraclesConfigData)
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

		// Define remaining accounts that will be passed to the receiver program
		remainingAccounts := []solana.PublicKey{
			reportState.PublicKey(),
		}
		for i := 0; i < 2; i++ {
			remainingAccount, err := solana.NewRandomPrivateKey()
			require.NoError(t, err)
			remainingAccounts = append(remainingAccounts, remainingAccount.PublicKey())
		}

		accountHash := generateAccountHash(forwarderStateAddress, forwarderAuthorityStorage, remainingAccounts)
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
		appendRemainingAccounts(fwdOnReportIx, remainingAccounts)
		fwdOnReportIxWithRemainingAccounts, err := fwdOnReportIx.ValidateAndBuild()
		require.NoError(t, err)

		res, err := common.SendAndConfirm(
			t.Context(),
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
		err = common.GetAccountDataBorshInto(t.Context(), solanaClient, executionStateStorage, rpc.CommitmentConfirmed, &executionState)
		require.NoError(t, err)
		require.Equal(t, true, executionState.Success)
		require.Equal(t, [32]byte(transmissionId), executionState.TransmissionId)
		require.Equal(t, deployerKey.PublicKey(), executionState.Transmitter)

		// check on the receiver end
		var latestReportAccount receiver_program.LatestReport
		err = common.GetAccountDataBorshInto(t.Context(), solanaClient, reportState.PublicKey(), rpc.CommitmentConfirmed, &latestReportAccount)
		require.NoError(t, err)
		require.Equal(t, payload, latestReportAccount.Report)
		require.Equal(t, forwarderAuthorityStorage, latestReportAccount.ForwarderAuthority)
		require.Equal(t, rawReportBytes[45:109], latestReportAccount.Metadata)

		// send the same report again, should fail with ExecutionAlreadySucceded error
		res, err = common.SendAndFailWith(
			t.Context(),
			solanaClient,
			[]solana.Instruction{fwdOnReportIxWithRemainingAccounts},
			deployerKey,
			rpc.CommitmentConfirmed,
			[]string{"Execution already succeded"},
			common.AddComputeUnitLimit(fees.ComputeUnitLimit(1_400_000)),
		)
		require.NoError(t, err)
	})

	t.Run("Report with Wrong Remaining Accounts", func(t *testing.T) {
		wrongAccountsReportState, err := solana.NewRandomPrivateKey()
		require.NoError(t, err)

		initializeReceiverProgram(t, wrongAccountsReportState, deployerKey, forwarderAuthorityStorage, solanaClient)

		// Create a random extra account that won't be included in the actual transaction
		extraAccount, err := solana.NewRandomPrivateKey()
		require.NoError(t, err)

		// Generate account hash with WRONG accounts (including extra account)
		wrongRemainingAccounts := []solana.PublicKey{
			wrongAccountsReportState.PublicKey(),
			extraAccount.PublicKey(), // This won't be in the actual transaction
		}
		accountHash := generateAccountHash(forwarderStateAddress, forwarderAuthorityStorage, wrongRemainingAccounts)

		wrongAccountsReportId := reportId + 2
		transmissionId := getTransmissionId(workflowExecutionId, wrongAccountsReportId, receiver_program_id)
		executionStateStorage, _, err := solana.FindProgramAddress(
			[][]byte{[]byte("execution_state"), forwarderStateAddress.Bytes(), transmissionId},
			keystone_forwarder.ProgramID,
		)
		require.NoError(t, err)

		signers := getFSigners(t, defaultSigners, F)
		payload := []byte{0xBB}
		dataBytes, _ := getDataBytes(t, accountHash, payload, wrongAccountsReportId, signers)

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

		// Only append the ACTUAL remaining accounts (not the extra one)
		actualRemainingAccounts := []solana.PublicKey{
			wrongAccountsReportState.PublicKey(),
		}
		appendRemainingAccounts(fwdOnReportIx, actualRemainingAccounts)
		fwdOnReportIxWithRemainingAccounts, err := fwdOnReportIx.ValidateAndBuild()
		require.NoError(t, err)

		// Should fail with InvalidAccountHash error because account hash includes
		// extraAccount but the actual transaction doesn't
		_, err = common.SendAndFailWith(
			t.Context(),
			solanaClient,
			[]solana.Instruction{fwdOnReportIxWithRemainingAccounts},
			deployerKey,
			rpc.CommitmentConfirmed,
			[]string{"Invalid Account Hash"},
			common.AddComputeUnitLimit(fees.ComputeUnitLimit(1_400_000)),
		)
		require.NoError(t, err)
	})

	t.Run("Report with Lookup Table", func(t *testing.T) {
		reportState, err := solana.NewRandomPrivateKey()
		require.NoError(t, err)

		initializeReceiverProgram(t, reportState, deployerKey, forwarderAuthorityStorage, solanaClient)

		// Create lookup table with random accounts
		numLookupAccounts := 10
		lookupAccounts := make([]solana.PublicKey, numLookupAccounts)
		for i := 0; i < numLookupAccounts; i++ {
			privKey, keyErr := solana.NewRandomPrivateKey()
			require.NoError(t, keyErr)
			lookupAccounts[i] = privKey.PublicKey()
		}

		// Setup the lookup table on-chain (retry up to 2 times on failure)
		// because this fails to find the correct slot sometimes.
		var lookupTableAddress solana.PublicKey
		for attempt := 0; attempt < 3; attempt++ {
			lookupTableAddress, err = setupLookupTableWithSafeSlot(t.Context(), solanaClient, deployerKey, lookupAccounts)
			if err == nil {
				break
			}
			if attempt < 2 {
				t.Logf("SetupLookupTable attempt %d failed: %v, retrying...", attempt+1, err)
				time.Sleep(500 * time.Millisecond)
			}
		}
		require.NoError(t, err)

		// Define remaining accounts - reportState first, then some accounts from lookup table
		remainingAccounts := []solana.PublicKey{
			reportState.PublicKey(),
		}
		// Add first 3 accounts from lookup table to remaining accounts
		remainingAccounts = append(remainingAccounts, lookupAccounts[:3]...)

		accountHash := generateAccountHash(forwarderStateAddress, forwarderAuthorityStorage, remainingAccounts)
		lookupReportId := reportId + 3
		transmissionId := getTransmissionId(workflowExecutionId, lookupReportId, receiver_program_id)
		executionStateStorage, _, err := solana.FindProgramAddress(
			[][]byte{[]byte("execution_state"), forwarderStateAddress.Bytes(), transmissionId},
			keystone_forwarder.ProgramID,
		)
		require.NoError(t, err)

		signers := getFSigners(t, defaultSigners, F)
		payload := []byte{255}
		dataBytes, rawReportBytes := getDataBytes(t, accountHash, payload, lookupReportId, signers)

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
		appendRemainingAccounts(fwdOnReportIx, remainingAccounts)
		fwdOnReportIxWithRemainingAccounts, err := fwdOnReportIx.ValidateAndBuild()
		require.NoError(t, err)

		// Create lookup tables map
		lookupTablesMap := make(map[solana.PublicKey]solana.PublicKeySlice)
		lookupTablesMap[lookupTableAddress] = lookupAccounts

		// Send transaction using lookup tables
		res, err := common.SendAndConfirmWithLookupTables(
			t.Context(),
			solanaClient,
			[]solana.Instruction{fwdOnReportIxWithRemainingAccounts},
			deployerKey,
			rpc.CommitmentConfirmed,
			lookupTablesMap, // Include our lookup table map
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
		err = common.GetAccountDataBorshInto(t.Context(), solanaClient, executionStateStorage, rpc.CommitmentConfirmed, &executionState)
		require.NoError(t, err)
		require.Equal(t, true, executionState.Success)

		// Verify the payload was received correctly
		var latestReportAccount receiver_program.LatestReport
		err = common.GetAccountDataBorshInto(t.Context(), solanaClient, reportState.PublicKey(), rpc.CommitmentConfirmed, &latestReportAccount)
		require.NoError(t, err)
		require.Equal(t, payload, latestReportAccount.Report)
		require.Equal(t, rawReportBytes[45:109], latestReportAccount.Metadata)
	})

	t.Run("Report by transmitter not equal to deployer", func(t *testing.T) {
		reportState, err := solana.NewRandomPrivateKey()
		require.NoError(t, err)

		initializeReceiverProgram(t, reportState, deployerKey, forwarderAuthorityStorage, solanaClient)

		// Define remaining accounts that will be passed to the receiver program
		remainingAccounts := []solana.PublicKey{
			reportState.PublicKey(),
		}

		accountHash := generateAccountHash(forwarderStateAddress, forwarderAuthorityStorage, remainingAccounts)
		diffTransmitterReportId := reportId + 4
		transmissionId := getTransmissionId(workflowExecutionId, diffTransmitterReportId, receiver_program_id)
		executionStateStorage, _, err := solana.FindProgramAddress(
			[][]byte{[]byte("execution_state"), forwarderStateAddress.Bytes(), transmissionId},
			keystone_forwarder.ProgramID,
		)
		require.NoError(t, err)

		signers := getFSigners(t, defaultSigners, F)
		payload := []byte{255}
		dataBytes, _ := getDataBytes(t, accountHash, payload, diffTransmitterReportId, signers)

		diffTransmitter := defaultTransmitters[1]
		soltesting.FundTestAccounts(t, []solana.PublicKey{diffTransmitter.privKey.PublicKey()}, solanaChain.URL)

		fwdOnReportIx := keystone_forwarder.NewReportInstruction(
			dataBytes,
			forwarderStateAddress,
			getOraclesConfigAddress(t, forwarderStateAddress, donId, configVersion),
			diffTransmitter.address,
			forwarderAuthorityStorage,
			executionStateStorage,
			receiver_program_id,
			solana.SystemProgramID,
		)
		appendRemainingAccounts(fwdOnReportIx, remainingAccounts)
		fwdOnReportIxWithRemainingAccounts, err := fwdOnReportIx.ValidateAndBuild()
		require.NoError(t, err)

		_, err = common.SendAndConfirm(
			t.Context(),
			solanaClient,
			[]solana.Instruction{fwdOnReportIxWithRemainingAccounts},
			diffTransmitter.privKey,
			rpc.CommitmentConfirmed,
			common.AddComputeUnitLimit(fees.ComputeUnitLimit(1_400_000)),
		)
		require.NoError(t, err)

		var executionState keystone_forwarder.ExecutionState
		err = common.GetAccountDataBorshInto(t.Context(), solanaClient, executionStateStorage, rpc.CommitmentConfirmed, &executionState)
		require.NoError(t, err)
		require.Equal(t, true, executionState.Success)
		require.Equal(t, [32]byte(transmissionId), executionState.TransmissionId)
		require.Equal(t, diffTransmitter.privKey.PublicKey(), executionState.Transmitter)
	})

	t.Run("Report by random transmitter", func(t *testing.T) {
		reportState, err := solana.NewRandomPrivateKey()
		require.NoError(t, err)

		initializeReceiverProgram(t, reportState, deployerKey, forwarderAuthorityStorage, solanaClient)

		// Define remaining accounts that will be passed to the receiver program
		remainingAccounts := []solana.PublicKey{
			reportState.PublicKey(),
		}

		accountHash := generateAccountHash(forwarderStateAddress, forwarderAuthorityStorage, remainingAccounts)
		randomTransmitterReportId := reportId + 5
		transmissionId := getTransmissionId(workflowExecutionId, randomTransmitterReportId, receiver_program_id)
		executionStateStorage, _, err := solana.FindProgramAddress(
			[][]byte{[]byte("execution_state"), forwarderStateAddress.Bytes(), transmissionId},
			keystone_forwarder.ProgramID,
		)
		require.NoError(t, err)

		signers := getFSigners(t, defaultSigners, F)
		payload := []byte{255}
		dataBytes, _ := getDataBytes(t, accountHash, payload, randomTransmitterReportId, signers)

		randomTransmitter, err := solana.NewRandomPrivateKey()
		require.NoError(t, err)
		soltesting.FundTestAccounts(t, []solana.PublicKey{randomTransmitter.PublicKey()}, solanaChain.URL)

		fwdOnReportIx := keystone_forwarder.NewReportInstruction(
			dataBytes,
			forwarderStateAddress,
			getOraclesConfigAddress(t, forwarderStateAddress, donId, configVersion),
			randomTransmitter.PublicKey(),
			forwarderAuthorityStorage,
			executionStateStorage,
			receiver_program_id,
			solana.SystemProgramID,
		)
		appendRemainingAccounts(fwdOnReportIx, remainingAccounts)
		fwdOnReportIxWithRemainingAccounts, err := fwdOnReportIx.ValidateAndBuild()
		require.NoError(t, err)

		_, err = common.SendAndConfirm(
			t.Context(),
			solanaClient,
			[]solana.Instruction{fwdOnReportIxWithRemainingAccounts},
			randomTransmitter,
			rpc.CommitmentConfirmed,
			common.AddComputeUnitLimit(fees.ComputeUnitLimit(1_400_000)),
		)
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

// hash the accounts together - forwarder state, forwarder authority, and all remaining accounts
func generateAccountHash(state solana.PublicKey, forwarderAuthority solana.PublicKey, remainingAccounts []solana.PublicKey) []byte {
	hasher := sha256.New()
	hasher.Write(state.Bytes())
	hasher.Write(forwarderAuthority.Bytes())
	for _, account := range remainingAccounts {
		hasher.Write(account.Bytes())
	}
	return hasher.Sum(nil)
}

// hash the workflow execution id, report id and receiver program id together
// this is dumb as well, any large values will break this byte placement.
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
			privKeys: privKey,
			address:  uint8Address,
		}
	}
	sort.Slice(signers, func(i, j int) bool {
		return bytes.Compare(signers[i].address[:], signers[j].address[:]) < 0
	})
	return signers
}

func generateTransmitters(t *testing.T, deployerKey solana.PrivateKey, n int) []Transmitter {
	transmitters := make([]Transmitter, n)

	// First, add the deployer key as it will be the transmitter
	transmitters[0] = Transmitter{
		privKey: deployerKey,
	}
	copy(transmitters[0].address[:], deployerKey.PublicKey().Bytes())

	// Generate random Solana key pairs for the rest
	for i := 1; i < n; i++ {
		privKey, err := solana.NewRandomPrivateKey()
		require.NoError(t, err)
		transmitters[i] = Transmitter{
			privKey: privKey,
		}
		copy(transmitters[i].address[:], privKey.PublicKey().Bytes())
	}

	// Sort transmitters in increasing order by address
	sort.Slice(transmitters, func(i, j int) bool {
		return bytes.Compare(transmitters[i].address[:], transmitters[j].address[:]) < 0
	})

	return transmitters
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
		t.Context(),
		solanaClient,
		[]solana.Instruction{receiverInit},
		deployerKey,
		rpc.CommitmentConfirmed,
		common.AddSigners(reportState),
	)
	require.NoError(t, err)
}

// appendRemainingAccounts appends remaining accounts to an instruction
// reportState should always be first in the remainingAccounts slice
func appendRemainingAccounts(ix *keystone_forwarder.Report, remainingAccounts []solana.PublicKey) {
	for _, account := range remainingAccounts {
		ix.Append(&solana.AccountMeta{
			PublicKey:  account,
			IsWritable: true,
			IsSigner:   false,
		})
	}
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
	rawReportBytes := make([]byte, MetadataLength+len(forwarderReportBytes))
	// metadata
	copy(rawReportBytes[:MetadataLength], metadataBytes)
	// forwarder report
	copy(rawReportBytes[MetadataLength:], forwarderReportBytes)

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
// this function is very very dumb, any large values will break this byte placement.
func buildRawReportBytes(reportId uint16) []byte {
	raw := make([]byte, MetadataLength)
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

func setupLookupTableWithSafeSlot(ctx context.Context, client *rpc.Client, admin solana.PrivateKey, entries []solana.PublicKey) (solana.PublicKey, error) {
	slot, err := client.GetSlot(ctx, rpc.CommitmentConfirmed)
	if err != nil {
		return solana.PublicKey{}, err
	}

	// common.SetupLookupTable uses (slot - 1). When slot is 0 this underflows to MaxUint64 and fails.
	createSlot := slot
	if slot > 1 {
		createSlot = slot - 1
	}

	table, createIx, err := common.NewCreateLookupTableInstruction(admin.PublicKey(), admin.PublicKey(), createSlot)
	if err != nil {
		return solana.PublicKey{}, err
	}

	_, err = common.SendAndConfirmWithLookupTablesAndRetries(
		ctx,
		client,
		[]solana.Instruction{createIx},
		admin,
		rpc.CommitmentConfirmed,
		map[solana.PublicKey]solana.PublicKeySlice{},
	)
	if err != nil {
		return solana.PublicKey{}, err
	}

	if err = common.ExtendLookupTable(ctx, client, table, admin, entries); err != nil {
		return solana.PublicKey{}, err
	}

	if err = common.AwaitSlotChange(ctx, client); err != nil {
		return solana.PublicKey{}, err
	}

	return table, nil
}
