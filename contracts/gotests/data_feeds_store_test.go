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

	"github.com/smartcontractkit/chainlink-solana/contracts/generated/data_feeds_proxy"
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

// TestDataFeedsStore reports through the forwarder into the data feeds store, which writes the
// round through to a DF1 store feed via its report callback. It pins the remaining account
// layout and account hash a workflow has to produce.
func TestDataFeedsStore(t *testing.T) {
	programIDs := map[string]string{
		"keystone_forwarder": keystone_forwarder.ProgramID.String(),
		"data_feeds_store":   data_feeds_store.ProgramID.String(),
		"store":              store.ProgramID.String(),
		"data_feeds_proxy":   data_feeds_proxy.ProgramID.String(),
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
		authority, _, err = solana.FindProgramAddress(
			[][]byte{[]byte("forwarder"), key.PublicKey().Bytes(), data_feeds_store.ProgramID.Bytes()},
			keystone_forwarder.ProgramID)
		require.NoError(t, err)
		return key.PublicKey(), authority
	}
	stagingForwarder, stagingAuthority := newForwarder(stagingF)
	mainnetForwarder, mainnetAuthority := newForwarder(mainnetF)

	// data feeds store, both forwarder authorities are allowed senders
	storeState, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	pda := func(seed string, key []byte) solana.PublicKey {
		addr, _, err := solana.FindProgramAddress([][]byte{[]byte(seed), storeState.PublicKey().Bytes(), key}, data_feeds_store.ProgramID)
		require.NoError(t, err)
		return addr
	}
	// data IDs key accounts zero padded to 32 bytes
	feedPDA := func(id [16]uint8) solana.PublicKey {
		var seed [32]byte
		copy(seed[:], id[:])
		return pda("feed", seed[:])
	}
	// events are CPI events: emitting instructions take the program's event authority and the program
	eventAuthority := func(program solana.PublicKey) solana.PublicKey {
		addr, _, err := solana.FindProgramAddress([][]byte{[]byte("__event_authority")}, program)
		require.NoError(t, err)
		return addr
	}
	storeEvents, df1Events := eventAuthority(data_feeds_store.ProgramID), eventAuthority(store.ProgramID)
	// the DF1 store's callbacks are signed by its own callback authority, the DF1 feeds' writer
	df1CallbackAuthority := pda("callback_authority", store.ProgramID.Bytes())

	send([]solana.Instruction{
		build(data_feeds_store.NewInitializeInstruction([]solana.PublicKey{deployer.PublicKey()}, 16, 3600,
			deployer.PublicKey(), storeState.PublicKey(), solana.SystemProgramID, storeEvents, data_feeds_store.ProgramID)),
		build(data_feeds_store.NewSetDefaultFeedMetadataInstruction([]data_feeds_store.WorkflowMetadata{
			{AllowedSender: stagingAuthority, AllowedWorkflowOwner: testWorkflowOwner, AllowedWorkflowName: testWorkflowName},
			{AllowedSender: mainnetAuthority, AllowedWorkflowOwner: testWorkflowOwner, AllowedWorkflowName: testWorkflowName},
			// the deployer sends reports directly to measure compute and heap per report
			{AllowedSender: deployer.PublicKey(), AllowedWorkflowOwner: testWorkflowOwner, AllowedWorkflowName: testWorkflowName},
		}, deployer.PublicKey(), storeState.PublicKey(), storeEvents, data_feeds_store.ProgramID)),
	}, storeState)
	// the owner allows the DF1 store as a callback program
	send([]solana.Instruction{
		build(data_feeds_store.NewSetCallbackProgramAllowedInstruction(store.ProgramID, true, deployer.PublicKey(),
			storeState.PublicKey(), pda("callback_program", store.ProgramID.Bytes()), solana.SystemProgramID,
			storeEvents, data_feeds_store.ProgramID)),
	})

	// target is a non-SVR feed and, when legacy is set, the DF1 feed it writes through to
	type target struct {
		id           [16]uint8
		feed, legacy solana.PublicKey
	}
	space := uint64(8 + 192 + 48)
	rent, err := client.GetMinimumBalanceForRentExemption(t.Context(), space, rpc.CommitmentConfirmed)
	require.NoError(t, err)
	newFeed := func(n byte, df1 bool) target {
		var id [16]uint8
		copy(id[:], []byte{0x00, 0x03, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, n})
		tg := target{id: id, feed: feedPDA(id)}
		send([]solana.Instruction{
			withAccounts(build(data_feeds_store.NewInitFeedsInstruction([][16]uint8{id}, deployer.PublicKey(), storeState.PublicKey(),
				solana.SystemProgramID, storeEvents, data_feeds_store.ProgramID)),
				solana.Meta(tg.feed).WRITE()),
		})
		if !df1 {
			return tg
		}
		// DF1 feed at 8 decimals, repointed to the DF1 store's callback authority, written through a callback
		legacy, err := solana.NewRandomPrivateKey()
		require.NoError(t, err)
		tg.legacy = legacy.PublicKey()
		send([]solana.Instruction{
			build(system.NewCreateAccountInstruction(rent, space, store.ProgramID, deployer.PublicKey(), tg.legacy).ValidateAndBuild()),
			build(store.NewCreateFeedInstruction("ETH / USD", 8, 1, 1, tg.legacy, deployer.PublicKey())),
			build(store.NewSetWriterInstruction(df1CallbackAuthority, tg.legacy, deployer.PublicKey(), deployer.PublicKey())),
		}, legacy)
		// a non-SVR data ID's callback lives in its feed account
		send([]solana.Instruction{
			withAccounts(build(data_feeds_store.NewSetReportCallbackConfigsInstruction([][16]uint8{id},
				[]data_feeds_store.CallbackConfigArgs{{
					Program: store.ProgramID,
					// the DF1 feed, then the DF1 store's event authority and program for its CPI events
					Accounts: []data_feeds_store.CallbackAccount{
						{Pubkey: tg.legacy, IsWritable: true}, {Pubkey: df1Events}, {Pubkey: store.ProgramID},
					},
				}}, deployer.PublicKey(), storeState.PublicKey(), solana.SystemProgramID, storeEvents, data_feeds_store.ProgramID)),
				solana.Meta(tg.feed).WRITE(), solana.Meta(store.ProgramID), solana.Meta(pda("callback_program", store.ProgramID.Bytes()))),
		})
		return tg
	}

	// report: Borsh Vec<ReceivedReport { data_id, timestamp: u32, answer: i128 }>
	answer := new(big.Int).Mul(big.NewInt(325012), new(big.Int).Exp(big.NewInt(10), big.NewInt(16), nil)) // 3250.12 at 18 decimals
	observedAt := uint32(time.Now().Unix() - 30)
	// payloadAndAccounts builds the report payload and the store's accounts after the forwarder's:
	// the store state, its event authority and program, then per report its feed and, when it
	// writes through to DF1, the callback program, its callback authority, the DF1 feed and the DF1
	// store's event authority and program.
	payloadAndAccounts := func(targets []target, observed uint32) ([]byte, []*solana.AccountMeta) {
		payload := make([]byte, 4+36*len(targets))
		binary.LittleEndian.PutUint32(payload, uint32(len(targets)))
		remaining := []*solana.AccountMeta{solana.Meta(storeState.PublicKey()), solana.Meta(storeEvents), solana.Meta(data_feeds_store.ProgramID)}
		for i, tg := range targets {
			o := 4 + 36*i
			copy(payload[o:], tg.id[:])
			binary.LittleEndian.PutUint32(payload[o+16:], observed)
			copy(payload[o+20:], littleEndianI128(answer))
			remaining = append(remaining, solana.Meta(tg.feed).WRITE())
			if !tg.legacy.IsZero() {
				remaining = append(remaining, solana.Meta(store.ProgramID), solana.Meta(df1CallbackAuthority),
					solana.Meta(tg.legacy).WRITE(), solana.Meta(df1Events), solana.Meta(store.ProgramID))
			}
		}
		return payload, remaining
	}
	// reportIx builds the forwarder report instruction a CRE write sends.
	reportIx := func(forwarderState, forwarderAuthority solana.PublicKey, f uint8, id uint16, targets []target) solana.Instruction {
		payload, remaining := payloadAndAccounts(targets, observedAt)
		keys := make([]solana.PublicKey, len(remaining))
		for i, m := range remaining {
			keys[i] = m.PublicKey
		}
		executionState, _, err := solana.FindProgramAddress([][]byte{[]byte("execution_state"),
			forwarderState.Bytes(), getTransmissionId(workflowExecutionId, id, data_feeds_store.ProgramID)}, keystone_forwarder.ProgramID)
		require.NoError(t, err)
		data, _ := getDataBytes(t, generateAccountHash(forwarderState, forwarderAuthority, keys), payload, id, getFSigners(t, signers, f))
		ix := build(keystone_forwarder.NewReportInstruction(data, forwarderState,
			getOraclesConfigAddress(t, forwarderState, donId, configVersion), deployer.PublicKey(),
			forwarderAuthority, executionState, data_feeds_store.ProgramID, solana.SystemProgramID))
		return withAccounts(ix, remaining...)
	}
	// sendLegacy sends a report the way CRE does: a legacy transaction with every account inline and
	// the relayer's compute unit limit and price instructions.
	sendLegacy := func(ix solana.Instruction, label string) {
		size := legacyTxSize(t, deployer, ix)
		t.Logf("legacy tx, %s: %d bytes", label, size)
		require.LessOrEqual(t, size, maxLegacyTxSize)
		res, err := common.SendAndConfirm(t.Context(), client, []solana.Instruction{ix}, deployer, rpc.CommitmentConfirmed,
			common.AddComputeUnitLimit(fees.ComputeUnitLimit(400_000)), common.AddComputeUnitPrice(fees.ComputeUnitPrice(1)))
		require.NoError(t, err)
		t.Logf("legacy tx, %s: %d compute units", label, *res.Meta.ComputeUnitsConsumed)
	}
	requireStored := func(tg target, roundID uint64) {
		// stored at 18 decimals
		info, err := client.GetAccountInfoWithOpts(t.Context(), tg.feed, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentConfirmed})
		require.NoError(t, err)
		feedData, err := data_feeds_store.ParseAccount_FeedData(info.Value.Data.GetBinary())
		require.NoError(t, err)
		require.Equal(t, roundID, feedData.RoundId)
		require.Equal(t, observedAt, feedData.LatestObservationsTimestamp)
		require.Equal(t, 0, answer.Cmp(feedData.LatestAnswer.BigInt()))
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
		require.Equal(t, observedAt, binary.LittleEndian.Uint32(live[8:12]))
		require.Equal(t, big.NewInt(325012000000).String(), fromLittleEndianI128(live[16:32]).String())
	}

	// F=2 (staging): one report written through to DF1 fits one legacy transaction. With CPI events
	// (the store's and the DF1 store's event authorities) a second one no longer does.
	df1 := []target{newFeed(1, true), newFeed(2, true)}
	sendLegacy(reportIx(stagingForwarder, stagingAuthority, stagingF, reportId+20, df1[:1]), "F=2, 1 report with DF1 write-through")
	requireStored(df1[0], 1)

	// A proxy in front of the store serves the same round, at the store's decimals and rescaled.
	proxyState, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	feedConfig, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("feed_config"), proxyState.PublicKey().Bytes(), df1[0].id[:]}, data_feeds_proxy.ProgramID)
	require.NoError(t, err)
	send([]solana.Instruction{
		build(data_feeds_proxy.NewInitializeInstruction(nil, deployer.PublicKey(), proxyState.PublicKey(),
			data_feeds_store.ProgramID, storeState.PublicKey(), solana.SystemProgramID)),
	}, proxyState)
	send([]solana.Instruction{
		build(data_feeds_proxy.NewSetMinDecimalsInstruction(df1[0].id, 8, deployer.PublicKey(), proxyState.PublicKey(),
			feedConfig, solana.SystemProgramID)),
	})
	simulateRound := func(label string, ix solana.Instruction) data_feeds_proxy.Round {
		tx, err := solana.NewTransaction([]solana.Instruction{ix}, solana.Hash{}, solana.TransactionPayer(deployer.PublicKey()))
		require.NoError(t, err)
		sim, err := client.SimulateTransactionWithOpts(t.Context(), tx, &rpc.SimulateTransactionOpts{
			SigVerify: false, ReplaceRecentBlockhash: true, Commitment: rpc.CommitmentConfirmed})
		require.NoError(t, err)
		require.Nil(t, sim.Value.Err, "logs: %v", sim.Value.Logs)
		require.NotNil(t, sim.Value.ReturnData)
		t.Logf("%s: %d compute units", label, *sim.Value.UnitsConsumed)
		var round data_feeds_proxy.Round
		require.NoError(t, round.UnmarshalWithDecoder(bin.NewBorshDecoder(sim.Value.ReturnData.Data.Content)))
		return round
	}
	direct := simulateRound("store read", build(data_feeds_store.NewGetLatestRoundInstruction(df1[0].id,
		storeState.PublicKey(), df1[0].feed)))
	require.Equal(t, uint64(1), direct.RoundId)
	atStoreDecimals := simulateRound("proxy read", build(data_feeds_proxy.NewReadInstruction(df1[0].id, nil, proxyState.PublicKey(),
		data_feeds_store.ProgramID, storeState.PublicKey(), df1[0].feed)))
	require.Equal(t, uint64(1), atStoreDecimals.RoundId)
	require.Equal(t, 0, answer.Cmp(atStoreDecimals.Answer.BigInt()))
	scaled := simulateRound("proxy read_scaled", build(data_feeds_proxy.NewReadScaledInstruction(df1[0].id, 8, nil, proxyState.PublicKey(),
		data_feeds_store.ProgramID, storeState.PublicKey(), df1[0].feed, feedConfig)))
	require.Equal(t, "325012000000", scaled.Answer.BigInt().String(), "3250.12 at 8 decimals")

	// F=5 (mainnet): one report fits without the DF1 write-through.
	storeOnly := newFeed(3, false)
	sendLegacy(reportIx(mainnetForwarder, mainnetAuthority, mainnetF, reportId+21, []target{storeOnly}), "F=5, 1 report")
	requireStored(storeOnly, 1)

	// F=2 (staging): two store-only reports fit.
	storeOnly2 := []target{newFeed(4, false), newFeed(5, false)}
	sendLegacy(reportIx(stagingForwarder, stagingAuthority, stagingF, reportId+24, storeOnly2), "F=2, 2 reports")
	for _, tg := range storeOnly2 {
		requireStored(tg, 1)
	}

	// A second DF1 write-through at F=2, or one at F=5, needs V1 transactions.
	size := legacyTxSize(t, deployer, reportIx(stagingForwarder, stagingAuthority, stagingF, reportId+22, df1))
	t.Logf("legacy tx, F=2, 2 reports with DF1 write-through: %d bytes", size)
	require.Greater(t, size, maxLegacyTxSize)
	size = legacyTxSize(t, deployer, reportIx(mainnetForwarder, mainnetAuthority, mainnetF, reportId+23, df1[:1]))
	t.Logf("legacy tx, F=5, 1 report with DF1 write-through: %d bytes", size)
	require.Greater(t, size, maxLegacyTxSize)

	// Compute and heap per report, without the forwarder's signatures so more reports fit one legacy
	// transaction: the deployer is an allowed sender and calls on_report directly. The heap is the
	// default 32KiB, so a heap overrun fails the transaction. Every report is accepted and, for DF1
	// targets, written through.
	metadata := make([]byte, 64)
	copy(metadata[32:42], testWorkflowName[:])
	copy(metadata[42:62], testWorkflowOwner[:])
	measure := func(targets []target, observed uint32, label string) {
		payload, remaining := payloadAndAccounts(targets, observed)
		ix := withAccounts(build(data_feeds_store.NewOnReportInstruction(metadata, payload, solana.SystemProgramID,
			deployer.PublicKey(), remaining[0].PublicKey, remaining[1].PublicKey, remaining[2].PublicKey)), remaining[3:]...)
		size := legacyTxSize(t, deployer, ix)
		res, err := common.SendAndConfirm(t.Context(), client, []solana.Instruction{ix}, deployer, rpc.CommitmentConfirmed,
			common.AddComputeUnitLimit(fees.ComputeUnitLimit(1_400_000)))
		require.NoError(t, err, label)
		t.Logf("direct on_report, %s: %d compute units, %d bytes", label, *res.Meta.ComputeUnitsConsumed, size)
	}
	var storeTargets, df1Targets []target
	for n := byte(0); n < 12; n++ {
		storeTargets = append(storeTargets, newFeed(0x40+n, false))
	}
	for n := byte(0); n < 6; n++ {
		df1Targets = append(df1Targets, newFeed(0x80+n, true))
	}
	for i, n := range []int{1, 6, 12} {
		measure(storeTargets[:n], observedAt+uint32(10+i), fmt.Sprintf("%d store-only reports", n))
	}
	for i, n := range []int{1, 3, 6} {
		measure(df1Targets[:n], observedAt+uint32(20+i), fmt.Sprintf("%d reports with DF1 write-through", n))
	}
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
