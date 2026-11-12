package keystone_forwarder_test

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"testing"
	"time"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/common"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/fees"

	ctf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	cldf_solana_provider "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana/provider"

	"github.com/smartcontractkit/chainlink-solana/contracts/generated/data_feeds_store"
	"github.com/smartcontractkit/chainlink-solana/contracts/generated/keystone_forwarder"
	"github.com/smartcontractkit/chainlink-solana/contracts/generated/store"
	soltesting "github.com/smartcontractkit/chainlink-solana/pkg/solana/testing"
)

// Workflow name and owner encoded by buildRawReportBytes.
var (
	testWorkflowName  = [10]uint8{9: 10}
	testWorkflowOwner = [20]uint8{19: 11}
)

// TestDataFeedsStore reports through the forwarder into the data feeds store, which creates feeds
// set by the feed admin, writes rounds into its feed accounts and keeps linked DF1 feeds
// written. It pins the account layout a workflow has to produce and measures transaction size,
// compute and heap.
func TestDataFeedsStore(t *testing.T) {
	programIDs := map[string]string{
		"keystone_forwarder": keystone_forwarder.ProgramID.String(),
		"data_feeds_store":   data_feeds_store.ProgramID.String(),
		"store":              store.ProgramID.String(),
	}
	provider := cldf_solana_provider.NewCTFChainProvider(t, 16423721717087811551,
		cldf_solana_provider.CTFChainProviderConfig{
			DeployerKeyGen:               cldf_solana_provider.PrivateKeyFromRaw(soltesting.Funder.String()),
			ProgramsPath:                 ProgramsPath,
			ProgramIDs:                   programIDs,
			WaitDelayAfterContainerStart: 5 * time.Second,
		},
	)
	node, err := provider.Initialize(t.Context())
	require.NoError(t, err)
	chain := node.(ctf_solana.Chain)
	client := rpc.New(chain.URL)
	deployer := *chain.DeployerKey
	send := func(ixs []solana.Instruction, signers ...solana.PrivateKey) *rpc.GetTransactionResult {
		res, err := common.SendAndConfirm(t.Context(), client, ixs, deployer, rpc.CommitmentConfirmed,
			common.AddSigners(signers...), common.AddComputeUnitLimit(fees.ComputeUnitLimit(1_400_000)))
		require.NoError(t, err)
		return res
	}
	build := func(ix solana.Instruction, err error) solana.Instruction {
		require.NoError(t, err)
		return ix
	}
	findPDA := func(program solana.PublicKey, seeds ...[]byte) solana.PublicKey {
		addr, _, err := solana.FindProgramAddress(seeds, program)
		require.NoError(t, err)
		return addr
	}
	// data IDs key accounts zero padded to 32 bytes
	seed32 := func(id [16]uint8) []byte {
		var seed [32]byte
		copy(seed[:], id[:])
		return seed[:]
	}

	// Two forwarder states of one DON's 16 signers, sized like the DF workflow DONs: F=2 as on
	// staging and F=5 as on mainnet. Each state has its own forwarder authority, as each zone does.
	signers := generateSigners(t, 16)
	addresses := make([][20]uint8, len(signers))
	for i, s := range signers {
		addresses[i] = s.address
	}
	const stagingF, mainnetF = uint8(2), uint8(5)
	newForwarder := func(f uint8) (state, authority solana.PublicKey) {
		key, err := solana.NewRandomPrivateKey()
		require.NoError(t, err)
		send([]solana.Instruction{
			build(keystone_forwarder.NewInitializeInstruction(key.PublicKey(), deployer.PublicKey(), solana.SystemProgramID)),
			build(keystone_forwarder.NewInitOraclesConfigInstruction(donId, configVersion, f, addresses, key.PublicKey(),
				getOraclesConfigAddress(t, key.PublicKey(), donId, configVersion), deployer.PublicKey(), solana.SystemProgramID)),
		}, key)
		return key.PublicKey(), findPDA(keystone_forwarder.ProgramID,
			[]byte("forwarder"), key.PublicKey().Bytes(), data_feeds_store.ProgramID.Bytes())
	}
	stagingForwarder, stagingAuthority := newForwarder(stagingF)
	mainnetForwarder, mainnetAuthority := newForwarder(mainnetF)

	storeState, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	storePDA := func(seed string, keys ...[]byte) solana.PublicKey {
		return findPDA(data_feeds_store.ProgramID, append([][]byte{[]byte(seed), storeState.PublicKey().Bytes()}, keys...)...)
	}
	df1Writer := storePDA("df1_writer")
	feedPDA := func(id [16]uint8) solana.PublicKey { return storePDA("feed", seed32(id)) }
	// events are CPI events: emitting instructions take the program's event authority and the program
	storeEvents := findPDA(data_feeds_store.ProgramID, []byte("__event_authority"))

	// A store lives in a pre-created state account that only the program's upgrade authority, here
	// the deployer, initializes.
	createState := func(key solana.PrivateKey, space uint64, program solana.PublicKey) solana.Instruction {
		rent, err := client.GetMinimumBalanceForRentExemption(t.Context(), space, rpc.CommitmentConfirmed)
		require.NoError(t, err)
		return build(system.NewCreateAccountInstruction(rent, space, program, deployer.PublicKey(), key.PublicKey()).ValidateAndBuild())
	}
	programData := func(program solana.PublicKey) solana.PublicKey {
		return findPDA(solana.BPFLoaderUpgradeableProgramID, program.Bytes())
	}
	send([]solana.Instruction{
		createState(storeState, 8+3232, data_feeds_store.ProgramID),
		build(data_feeds_store.NewInitializeInstruction(data_feeds_store.StoreConfig{
			FeedAdmins:                []solana.PublicKey{deployer.PublicKey()},
			DefaultMaxStoredReports:   16,
			MaxObservationForwardSecs: 3600,
			Df1Program:                store.ProgramID,
		}, deployer.PublicKey(), storeState.PublicKey(), programData(data_feeds_store.ProgramID), storeEvents, data_feeds_store.ProgramID)),
	}, storeState)
	send([]solana.Instruction{
		build(data_feeds_store.NewSetDefaultFeedMetadataInstruction([]data_feeds_store.WorkflowMetadata{
			{AllowedSender: stagingAuthority, AllowedWorkflowOwner: testWorkflowOwner, AllowedWorkflowName: testWorkflowName},
			{AllowedSender: mainnetAuthority, AllowedWorkflowOwner: testWorkflowOwner, AllowedWorkflowName: testWorkflowName},
			// the deployer sends reports directly to measure compute and heap per report
			{AllowedSender: deployer.PublicKey(), AllowedWorkflowOwner: testWorkflowOwner, AllowedWorkflowName: testWorkflowName},
		}, deployer.PublicKey(), storeState.PublicKey(), storeEvents, data_feeds_store.ProgramID)),
	})

	// target is a non-SVR data ID and, when legacy is set, the DF1 feed it is linked to
	type target struct {
		id     [16]uint8
		legacy solana.PublicKey
	}
	dataID := func(prefix byte, n byte) [16]uint8 {
		var id [16]uint8
		copy(id[:], []byte{0x00, 0x03, 0xaa, 0xbb, 0xcc, 0xdd, prefix, n})
		return id
	}
	df1Space := uint64(8 + 192 + 48)
	df1Rent, err := client.GetMinimumBalanceForRentExemption(t.Context(), df1Space, rpc.CommitmentConfirmed)
	require.NoError(t, err)
	// feed admin calls; remaining accounts: the feeds
	adminIx := func(ix solana.Instruction, err error, ids [][16]uint8, extra ...*solana.AccountMeta) solana.Instruction {
		feeds := make([]*solana.AccountMeta, len(ids))
		for i, id := range ids {
			feeds[i] = solana.Meta(feedPDA(id)).WRITE()
		}
		return withAccounts(build(ix, err), append(feeds, extra...)...)
	}
	setFeeds := func(ids ...[16]uint8) solana.Instruction {
		descriptions := make([]string, len(ids))
		for i := range ids {
			descriptions[i] = fmt.Sprintf("F%02d / USD", i)
		}
		ix, err := data_feeds_store.NewSetFeedsInstruction(ids, descriptions, nil, deployer.PublicKey(), storeState.PublicKey(),
			solana.SystemProgramID, storeEvents, data_feeds_store.ProgramID)
		return adminIx(ix, err, ids)
	}
	// newDF1Target sets a feed and links it to a new DF1 feed at 8 decimals whose writer is the
	// store's df1_writer
	newDF1Target := func(id [16]uint8) target {
		legacy, err := solana.NewRandomPrivateKey()
		require.NoError(t, err)
		ix, err := data_feeds_store.NewSetDf1FeedsInstruction([][16]uint8{id}, []solana.PublicKey{legacy.PublicKey()}, []bool{false},
			deployer.PublicKey(), storeState.PublicKey(), solana.SystemProgramID, storeEvents, data_feeds_store.ProgramID)
		send([]solana.Instruction{
			build(system.NewCreateAccountInstruction(df1Rent, df1Space, store.ProgramID, deployer.PublicKey(), legacy.PublicKey()).ValidateAndBuild()),
			build(store.NewCreateFeedInstruction("ETH / USD", 8, 1, 1, legacy.PublicKey(), deployer.PublicKey())),
			build(store.NewSetWriterInstruction(df1Writer, legacy.PublicKey(), deployer.PublicKey(), deployer.PublicKey())),
			setFeeds(id),
			adminIx(ix, err, [][16]uint8{id}, solana.Meta(legacy.PublicKey())),
		}, legacy)
		return target{id: id, legacy: legacy.PublicKey()}
	}

	// report: Borsh Vec<ReceivedReport { data_id, timestamp: u32, answer: i128 }>
	answer := new(big.Int).Mul(big.NewInt(325012), new(big.Int).Exp(big.NewInt(10), big.NewInt(16), nil)) // 3250.12 at 18 decimals
	observedAt := uint32(time.Now().Unix() - 300)
	// payloadAndAccounts builds the report payload and the store's accounts after the forwarder's:
	// the store state, the DF1 program and writer when a report writes DF1 (the store program stands
	// for them otherwise), the event authority and program, then per report its feed and its linked
	// DF1 feed.
	payloadAndAccounts := func(targets []target, observed uint32) ([]byte, []*solana.AccountMeta) {
		none := solana.Meta(data_feeds_store.ProgramID)
		withDF1 := false
		for _, tg := range targets {
			withDF1 = withDF1 || !tg.legacy.IsZero()
		}
		remaining := []*solana.AccountMeta{
			solana.Meta(storeState.PublicKey()), none, none,
			solana.Meta(storeEvents), solana.Meta(data_feeds_store.ProgramID),
		}
		if withDF1 {
			remaining[1], remaining[2] = solana.Meta(store.ProgramID), solana.Meta(df1Writer)
		}
		payload := make([]byte, 4+36*len(targets))
		binary.LittleEndian.PutUint32(payload, uint32(len(targets)))
		for i, tg := range targets {
			o := 4 + 36*i
			copy(payload[o:], tg.id[:])
			binary.LittleEndian.PutUint32(payload[o+16:], observed)
			copy(payload[o+20:], littleEndianI128(answer))
			remaining = append(remaining, solana.Meta(feedPDA(tg.id)).WRITE())
			if !tg.legacy.IsZero() {
				remaining = append(remaining, solana.Meta(tg.legacy).WRITE())
			}
		}
		return payload, remaining
	}
	// reportIx builds the forwarder report instruction a CRE write sends.
	reportIx := func(forwarderState, forwarderAuthority solana.PublicKey, f uint8, id uint16, targets []target, observed uint32) solana.Instruction {
		payload, remaining := payloadAndAccounts(targets, observed)
		keys := make([]solana.PublicKey, len(remaining))
		for i, m := range remaining {
			keys[i] = m.PublicKey
		}
		executionState := findPDA(keystone_forwarder.ProgramID, []byte("execution_state"),
			forwarderState.Bytes(), getTransmissionId(workflowExecutionId, id, data_feeds_store.ProgramID))
		data, _ := getDataBytes(t, generateAccountHash(forwarderState, forwarderAuthority, keys), payload, id, getFSigners(t, signers, f))
		ix := build(keystone_forwarder.NewReportInstruction(data, forwarderState,
			getOraclesConfigAddress(t, forwarderState, donId, configVersion), deployer.PublicKey(),
			forwarderAuthority, executionState, data_feeds_store.ProgramID, solana.SystemProgramID))
		return withAccounts(ix, remaining...)
	}
	// directIx calls on_report with the deployer as sender, without the forwarder's signatures.
	metadata := make([]byte, 64)
	copy(metadata[32:42], testWorkflowName[:])
	copy(metadata[42:62], testWorkflowOwner[:])
	directIx := func(targets []target, observed uint32) solana.Instruction {
		payload, remaining := payloadAndAccounts(targets, observed)
		ix := build(data_feeds_store.NewOnReportInstruction(metadata, payload, solana.SystemProgramID, deployer.PublicKey(),
			storeState.PublicKey(), data_feeds_store.ProgramID, data_feeds_store.ProgramID, storeEvents, data_feeds_store.ProgramID))
		generic := ix.(*solana.GenericInstruction)
		generic.AccountValues = append([]*solana.AccountMeta{solana.Meta(solana.SystemProgramID), solana.Meta(deployer.PublicKey()).SIGNER()}, remaining...)
		return generic
	}
	// lookupTable puts every account of the instructions (but the payer) in a new address lookup
	// table. Test harness only: CRE writes won't use lookup tables but V1 transactions (up to 4,096
	// bytes with every account inline), which neither this validator nor solana-go sends yet. A table
	// is just how a batch larger than a legacy transaction gets on chain here, to measure its compute
	// and heap; the inline size each send logs is what its V1 transaction would carry.
	lookupTable := func(ixs ...solana.Instruction) map[solana.PublicKey]solana.PublicKeySlice {
		seen := map[solana.PublicKey]bool{deployer.PublicKey(): true}
		var entries solana.PublicKeySlice
		for _, ix := range ixs {
			for _, m := range append(ix.Accounts(), solana.Meta(ix.ProgramID())) {
				if !seen[m.PublicKey] {
					seen[m.PublicKey] = true
					entries = append(entries, m.PublicKey)
				}
			}
		}
		// the helper derives the table from the finalized slot minus one, so wait for one on a new chain
		require.Eventually(t, func() bool {
			slot, err := client.GetSlot(t.Context(), rpc.CommitmentFinalized)
			return err == nil && slot > 1
		}, time.Minute, 500*time.Millisecond)
		table, err := common.CreateLookupTable(t.Context(), client, deployer)
		require.NoError(t, err)
		for start := 0; start < len(entries); start += 20 {
			end := min(start+20, len(entries))
			require.NoError(t, common.ExtendLookupTable(t.Context(), client, table, deployer, entries[start:end]))
		}
		require.NoError(t, common.AwaitSlotChange(t.Context(), client))
		return map[solana.PublicKey]solana.PublicKeySlice{table: entries}
	}
	// sendReport sends an instruction as a legacy transaction with every account inline when it fits
	// 1,232 bytes, and through a test-only lookup table otherwise. It logs the inline size either way:
	// what a V1 transaction (up to 4,096 bytes, no lookup table) would carry.
	sendReport := func(ix solana.Instruction, label string) *rpc.GetTransactionResult {
		size := legacyTxSize(t, deployer, ix)
		// local validators enable the feature gate raising the account lock limit to 128, mainnet
		// hasn't: every transaction is checked against 64 here, lookup table or not
		accounts := txAccountCount(deployer, ix)
		require.LessOrEqual(t, accounts, maxTxAccounts, "%s locks %d accounts", label, accounts)
		var res *rpc.GetTransactionResult
		var err error
		if size <= maxLegacyTxSize {
			res, err = common.SendAndConfirm(t.Context(), client, []solana.Instruction{ix}, deployer, rpc.CommitmentConfirmed,
				common.AddComputeUnitLimit(fees.ComputeUnitLimit(1_400_000)), common.AddComputeUnitPrice(fees.ComputeUnitPrice(1)))
		} else {
			res, err = common.SendAndConfirmWithLookupTables(t.Context(), client, []solana.Instruction{ix}, deployer,
				rpc.CommitmentConfirmed, lookupTable(ix), common.AddComputeUnitLimit(fees.ComputeUnitLimit(1_400_000)))
		}
		require.NoError(t, err, label)
		t.Logf("%s: %d bytes inline (legacy fits: %v), %d accounts, %d compute units", label, size, size <= maxLegacyTxSize,
			accounts, *res.Meta.ComputeUnitsConsumed)
		return res
	}

	requireStored := func(tg target, roundID uint64, observed uint32) {
		// stored in the feed at 18 decimals
		info, err := client.GetAccountInfoWithOpts(t.Context(), feedPDA(tg.id), &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentConfirmed})
		require.NoError(t, err)
		feed, err := data_feeds_store.ParseAccount_Feed(info.Value.Data.GetBinary())
		require.NoError(t, err)
		require.Equal(t, roundID, feed.RoundId)
		require.Equal(t, observed, feed.LatestObservationsTimestamp)
		require.Equal(t, 0, answer.Cmp(feed.LatestAnswer.BigInt()))
		if tg.legacy.IsZero() {
			return
		}
		// written through to the DF1 feed at its own 8 decimals
		info, err = client.GetAccountInfoWithOpts(t.Context(), tg.legacy, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentConfirmed})
		require.NoError(t, err)
		legacyData := info.Value.Data.GetBinary()
		transmissions, err := store.ParseAccount_Transmissions(legacyData)
		require.NoError(t, err)
		require.Equal(t, uint32(roundID), transmissions.LatestRoundId)
		live := legacyData[8+192 : 8+192+48] // slot u64, timestamp u32, padding u32, answer i128
		require.Equal(t, observed, binary.LittleEndian.Uint32(live[8:12]))
		require.Equal(t, big.NewInt(325012000000).String(), fromLittleEndianI128(live[16:32]).String())
	}

	// Through the forwarder: a store-only feed and a DF1-linked feed, set by the feed admin, written
	// at F=2 (staging) then F=5 (mainnet).
	storeOnly := target{id: dataID(0xee, 1)}
	send([]solana.Instruction{setFeeds(storeOnly.id)})
	df1Target := newDF1Target(dataID(0xee, 2))
	sendReport(reportIx(stagingForwarder, stagingAuthority, stagingF, reportId+20, []target{storeOnly}, observedAt),
		"forwarder F=2, 1 store-only report")
	requireStored(storeOnly, 1, observedAt)
	sendReport(reportIx(stagingForwarder, stagingAuthority, stagingF, reportId+21, []target{df1Target}, observedAt),
		"forwarder F=2, 1 report with DF1 write-through")
	requireStored(df1Target, 1, observedAt)
	sendReport(reportIx(mainnetForwarder, mainnetAuthority, mainnetF, reportId+22, []target{storeOnly}, observedAt+1),
		"forwarder F=5, 1 store-only report")
	requireStored(storeOnly, 2, observedAt+1)
	sendReport(reportIx(mainnetForwarder, mainnetAuthority, mainnetF, reportId+23, []target{df1Target}, observedAt+1),
		"forwarder F=5, 1 report with DF1 write-through")
	requireStored(df1Target, 2, observedAt+1)

	// Readers call the store, or decode the feed account: at 18 decimals, and at 8 once a feed
	// admin lowers the feed's minimum.
	simulateRound := func(label string, ix solana.Instruction) data_feeds_store.Round {
		tx, err := solana.NewTransaction([]solana.Instruction{ix}, solana.Hash{}, solana.TransactionPayer(deployer.PublicKey()))
		require.NoError(t, err)
		sim, err := client.SimulateTransactionWithOpts(t.Context(), tx, &rpc.SimulateTransactionOpts{
			SigVerify: false, ReplaceRecentBlockhash: true, Commitment: rpc.CommitmentConfirmed})
		require.NoError(t, err)
		require.Nil(t, sim.Value.Err, "logs: %v", sim.Value.Logs)
		require.NotNil(t, sim.Value.ReturnData)
		t.Logf("%s: %d compute units", label, *sim.Value.UnitsConsumed)
		var round data_feeds_store.Round
		require.NoError(t, round.UnmarshalWithDecoder(bin.NewBorshDecoder(sim.Value.ReturnData.Data.Content)))
		return round
	}
	atStored := simulateRound("latest_round at 18 decimals", build(data_feeds_store.NewLatestRoundInstruction(df1Target.id, 18,
		storeState.PublicKey(), feedPDA(df1Target.id))))
	require.Equal(t, uint64(2), atStored.RoundId)
	require.Equal(t, 0, answer.Cmp(atStored.Answer.BigInt()))
	minIx, err := data_feeds_store.NewSetMinDecimalsInstruction([][16]uint8{df1Target.id}, 8, deployer.PublicKey(),
		storeState.PublicKey(), solana.SystemProgramID, storeEvents, data_feeds_store.ProgramID)
	send([]solana.Instruction{adminIx(minIx, err, [][16]uint8{df1Target.id})})
	scaled := simulateRound("latest_round at 8 decimals", build(data_feeds_store.NewLatestRoundInstruction(df1Target.id, 8,
		storeState.PublicKey(), feedPDA(df1Target.id))))
	require.Equal(t, "325012000000", scaled.Answer.BigInt().String(), "3250.12 at 8 decimals")

	// Admin batches: 20 feeds set (created, with descriptions), then given specific metadata.
	var adminIDs [][16]uint8
	for n := byte(0); n < 20; n++ {
		adminIDs = append(adminIDs, dataID(0xc0, n))
	}
	sendReport(setFeeds(adminIDs...), "set_feeds, 20 feeds created")
	metadataIx, err := data_feeds_store.NewSetSpecificFeedMetadataInstruction(adminIDs,
		[]data_feeds_store.WorkflowMetadata{{AllowedSender: stagingAuthority, AllowedWorkflowOwner: testWorkflowOwner,
			AllowedWorkflowName: testWorkflowName}},
		deployer.PublicKey(), storeState.PublicKey(), solana.SystemProgramID, storeEvents, data_feeds_store.ProgramID)
	sendReport(adminIx(metadataIx, err, adminIDs), "set_specific_feed_metadata, 20 feeds")

	// Compute and heap per report, without the forwarder's signatures: the deployer is an allowed
	// sender and calls on_report directly. The heap is the default 32KiB, so a heap overrun fails the
	// transaction; every send is checked against 64 accounts.
	var storeTargets, df1Targets []target
	for n := byte(0); n < 40; n++ {
		storeTargets = append(storeTargets, target{id: dataID(0x40, n)})
	}
	for start := 0; start < len(storeTargets); start += 20 {
		ids := make([][16]uint8, 0, 20)
		for _, tg := range storeTargets[start:min(start+20, len(storeTargets))] {
			ids = append(ids, tg.id)
		}
		sendReport(setFeeds(ids...), fmt.Sprintf("set_feeds, %d feeds", len(ids)))
	}
	for n := byte(0); n < 25; n++ {
		df1Targets = append(df1Targets, newDF1Target(dataID(0x80, n)))
	}
	// a transaction the local validator accepts (1,232 bytes, even with a lookup table) carries the
	// report data of about 20 reports; larger batches are only measured below, for V1
	for i, n := range []int{1, 12, 20} {
		sendReport(directIx(storeTargets[:n], observedAt+uint32(10+i)), fmt.Sprintf("direct on_report, %d store-only reports", n))
	}
	for i, n := range []int{1, 12, 20} {
		sendReport(directIx(df1Targets[:n], observedAt+uint32(20+i)), fmt.Sprintf("direct on_report, %d reports with DF1 write-through", n))
	}
	requireStored(df1Targets[0], 3, observedAt+22)
	// forwarder reports sized for a V1 transaction (4,096 bytes, no lookup table) at F=5
	for _, c := range []struct {
		tgs   []target
		label string
	}{
		{storeTargets[:30], "30 store-only reports"},
		{storeTargets[:40], "40 store-only reports"},
		{df1Targets[:20], "20 reports with DF1 write-through"},
		{df1Targets, "25 reports with DF1 write-through"},
	} {
		ix := reportIx(mainnetForwarder, mainnetAuthority, mainnetF, reportId+31, c.tgs, observedAt+30)
		t.Logf("forwarder F=5, %s: %d bytes inline (V1 limit 4096), %d accounts", c.label,
			legacyTxSize(t, deployer, ix), txAccountCount(deployer, ix))
	}
}

// maxTxAccounts is the mainnet limit on the accounts a transaction locks.
const maxTxAccounts = 64

// txAccountCount is the number of distinct accounts a transaction carrying ix with the relayer's
// compute budget instructions locks: its accounts, its program, the payer and the compute budget
// program.
func txAccountCount(payer solana.PrivateKey, ix solana.Instruction) int {
	seen := map[solana.PublicKey]bool{payer.PublicKey(): true, solana.ComputeBudget: true, ix.ProgramID(): true}
	for _, m := range ix.Accounts() {
		seen[m.PublicKey] = true
	}
	return len(seen)
}

// maxLegacyTxSize is the Solana packet limit on a serialized transaction.
const maxLegacyTxSize = 1232

// legacyTxSize is the serialized size of a legacy transaction carrying ix with the compute unit
// limit and price instructions the relayer adds.
func legacyTxSize(t *testing.T, payer solana.PrivateKey, ix solana.Instruction) int {
	tx, err := solana.NewTransaction([]solana.Instruction{ix}, solana.Hash{}, solana.TransactionPayer(payer.PublicKey()))
	require.NoError(t, err)
	require.NoError(t, fees.SetComputeUnitLimit(tx, fees.ComputeUnitLimit(300_000)))
	require.NoError(t, fees.SetComputeUnitPrice(tx, fees.ComputeUnitPrice(1)))
	_, err = tx.Sign(func(solana.PublicKey) *solana.PrivateKey { return &payer })
	require.NoError(t, err)
	raw, err := tx.MarshalBinary()
	require.NoError(t, err)
	return len(raw)
}

func withAccounts(ix solana.Instruction, accounts ...*solana.AccountMeta) solana.Instruction {
	generic := ix.(*solana.GenericInstruction)
	generic.AccountValues = append(generic.AccountValues, accounts...)
	return generic
}

func littleEndianI128(v *big.Int) []byte {
	twos := new(big.Int).Set(v)
	if v.Sign() < 0 {
		twos.Add(twos, new(big.Int).Lsh(big.NewInt(1), 128))
	}
	be := twos.FillBytes(make([]byte, 16))
	out := make([]byte, 16)
	for i := range be {
		out[i] = be[15-i]
	}
	return out
}

func fromLittleEndianI128(le []byte) *big.Int {
	be := make([]byte, 16)
	for i := range le {
		be[i] = le[15-i]
	}
	v := new(big.Int).SetBytes(be)
	if be[0]&0x80 != 0 {
		v.Sub(v, new(big.Int).Lsh(big.NewInt(1), 128))
	}
	return v
}
