import * as anchor from "@coral-xyz/anchor";
import { Program, BN } from "@coral-xyz/anchor";
import { AccountMeta, Keypair, PublicKey, SystemProgram } from "@solana/web3.js";
import { assert } from "chai";
import { randomBytes } from "crypto";
import { DataFeedsStore } from "../target/types/data_feeds_store";
import { KeystoneForwarder } from "../target/types/keystone_forwarder";
import { Store } from "../target/types/store";
import {
  calculateForwarderAuthorityBump,
  Forwarder,
  newSigners,
  Signer,
} from "./utils";

const E18 = new BN(10).pow(new BN(18));
const E8 = new BN(10).pow(new BN(8));

// A transaction may lock at most 64 accounts on mainnet (local validators enable a feature gate
// raising it to 128, so the limit is checked here instead).
const MAX_TX_ACCOUNTS = 64;

type Report = { dataId: Buffer; timestamp: number; answer: BN };

// Borsh Vec<ReceivedReport { data_id: [u8; 16], timestamp: u32, answer: i128 }>
const encodeReports = (reports: Report[]) => {
  const out = Buffer.alloc(4 + reports.length * 36);
  out.writeUInt32LE(reports.length, 0);
  reports.forEach((r, i) => {
    const o = 4 + i * 36;
    r.dataId.copy(out, o);
    out.writeUInt32LE(r.timestamp, o + 16);
    r.answer
      .toTwos(128)
      .toArrayLike(Buffer, "le", 16)
      .copy(out, o + 20);
  });
  return out;
};

// compact feed id (8) | index (1) | svr path id (2, reserved) | instance (1) | reserved (4)
const newDataId = () => {
  const id = Buffer.alloc(16);
  randomBytes(8).copy(id, 0);
  return id;
};
const withSvrPath = (id: Buffer, path: number) => {
  const out = Buffer.from(id);
  out.writeUInt16BE(path, 9);
  return out;
};
// accounts key data ids zero padded to 32 bytes
const seed32 = (id: Buffer) => Buffer.concat([id, Buffer.alloc(16)]);

describe("data feeds store", function () {
  this.timeout(90_000);
  const provider = anchor.AnchorProvider.env();
  anchor.setProvider(provider);
  const connection = provider.connection;

  const program = anchor.workspace.DataFeedsStore as Program<DataFeedsStore>;
  const forwarderProgram = anchor.workspace
    .KeystoneForwarder as Program<KeystoneForwarder>;
  const df1 = anchor.workspace.Store as Program<Store>;

  let feedAdmin: Signer;
  let forwarder: Forwarder;
  let otherForwarder: Forwarder;
  let forwarderAuthority: PublicKey;
  let state: Keypair;
  const workflowOwner = randomBytes(20);
  const workflowName = randomBytes(10);

  const df1Writer = () =>
    PublicKey.findProgramAddressSync(
      [Buffer.from("df1_writer"), state.publicKey.toBuffer()],
      program.programId
    )[0];
  const feedPda = (id: Buffer) =>
    PublicKey.findProgramAddressSync(
      [Buffer.from("feed"), state.publicKey.toBuffer(), seed32(id)],
      program.programId
    )[0];
  const eventAuthority = (programId: PublicKey) =>
    PublicKey.findProgramAddressSync(
      [Buffer.from("__event_authority")],
      programId
    )[0];
  const eventAccounts = (programId = program.programId) => ({
    eventAuthority: eventAuthority(programId),
    program: programId,
  });
  const meta = (pubkey: PublicKey, isWritable = true): AccountMeta => ({
    pubkey,
    isSigner: false,
    isWritable,
  });

  // observation timestamps close to the validator clock
  let baseTimestamp: number;
  let tick = 0;
  const nextTimestamp = () => baseTimestamp + ++tick;

  const loader = new PublicKey("BPFLoaderUpgradeab1e11111111111111111111111");
  const programData = (programId: PublicKey) =>
    PublicKey.findProgramAddressSync([programId.toBuffer()], loader)[0];
  const createAccount = async (
    account: Keypair,
    space: number,
    programId: PublicKey
  ) =>
    SystemProgram.createAccount({
      fromPubkey: provider.publicKey,
      newAccountPubkey: account.publicKey,
      space,
      lamports: await connection.getMinimumBalanceForRentExemption(space),
      programId,
    });
  const initializeStore = (owner: PublicKey, account: PublicKey) =>
    program.methods
      .initialize({
        feedAdmins: [feedAdmin.keypair.publicKey],
        defaultMaxStoredReports: new BN(4),
        maxObservationForwardSecs: 3600,
        df1Program: df1.programId,
      })
      .accountsStrict({
        owner,
        state: account,
        programData: programData(program.programId),
        ...eventAccounts(),
      });
  const workflow = (name = workflowName) => ({
    allowedSender: forwarderAuthority,
    allowedWorkflowOwner: Array.from(workflowOwner),
    allowedWorkflowName: Array.from(name),
  });
  const setDefault = () =>
    program.methods
      .setDefaultFeedMetadata([workflow()])
      .accountsStrict({
        feedAdmin: feedAdmin.keypair.publicKey,
        state: state.publicKey,
        ...eventAccounts(),
      })
      .signers([feedAdmin.keypair])
      .rpc();

  // a new store with the forwarder as default workflow
  const initStore = async () => {
    state = Keypair.generate();
    await initializeStore(provider.publicKey, state.publicKey)
      .preInstructions([
        await createAccount(
          state,
          program.account.storeState.size,
          program.programId
        ),
      ])
      .signers([state])
      .rpc();
    await setDefault();
  };

  // feed admin calls; remaining accounts: the feeds
  const adminAccounts = (admin = feedAdmin.keypair.publicKey) => ({
    feedAdmin: admin,
    state: state.publicKey,
    systemProgram: SystemProgram.programId,
    ...eventAccounts(),
  });
  const setFeeds = (
    ids: Buffer[],
    descriptions = ids.map(() => ""),
    workflows: ReturnType<typeof workflow>[] = []
  ) =>
    program.methods
      .setFeeds(
        ids.map((id) => Array.from(id)),
        descriptions,
        workflows
      )
      .accountsStrict(adminAccounts())
      .remainingAccounts(ids.map((id) => meta(feedPda(id))))
      .signers([feedAdmin.keypair])
      .rpc();

  // Fixed accounts after the forwarder's: the store state, the DF1 program and writer (the program
  // id when no report writes DF1), the event authority and program. Per report: the feed, then the
  // linked DF1 feed.
  type ReportOpts = {
    df1Feeds?: Map<string, PublicKey>;
    passDf1?: boolean;
    via?: Forwarder;
    accounts?: (r: Report) => AccountMeta[];
  };
  const reportAccounts = (reports: Report[], opts: ReportOpts = {}) => {
    const none = meta(program.programId, false);
    const passDf1 = opts.passDf1 ?? opts.df1Feeds !== undefined;
    const accounts: AccountMeta[] = [
      meta(state.publicKey, false),
      passDf1 ? meta(df1.programId, false) : none,
      passDf1 ? meta(df1Writer(), false) : none,
      meta(eventAuthority(program.programId), false),
      meta(program.programId, false),
    ];
    for (const r of reports) {
      if (opts.accounts) {
        accounts.push(...opts.accounts(r));
        continue;
      }
      accounts.push(meta(feedPda(r.dataId)));
      const linked = opts.df1Feeds?.get(r.dataId.toString("hex"));
      if (linked) accounts.push(meta(linked));
    }
    return accounts;
  };
  const report = async (reports: Report[], opts: ReportOpts = {}) => {
    const via = opts.via ?? forwarder;
    const accounts = reportAccounts(reports, opts);
    // the forwarder's own accounts, the compute budget program and the payer come on top
    const keys = new Set(accounts.map((a) => a.pubkey.toBase58()));
    assert.isAtMost(
      keys.size + 8,
      MAX_TX_ACCOUNTS,
      "more accounts than a mainnet transaction can lock"
    );
    const { result } = await via.report(
      program.programId,
      encodeReports(reports),
      workflowName,
      workflowOwner,
      accounts
    );
    return result;
  };

  // reads, as on Stellar's proxy
  const read = (id: Buffer) => ({ state: state.publicKey, feed: feedPda(id) });
  const readLatest = async (id: Buffer, decimals = 18) =>
    (await program.methods
      .latestRound(Array.from(id), decimals)
      .accountsStrict(read(id))
      .view())!;
  const latestAnswer = async (id: Buffer) => (await readLatest(id)).answer as BN;
  const exists = async (key: PublicKey) =>
    (await connection.getAccountInfo(key)) !== null;

  // CPI events of the store in a transaction, decoded from its inner instructions
  const cpiEvents = async (signature: string) => {
    let tx = null;
    for (let i = 0; i < 20 && !tx; i++) {
      tx = await connection.getTransaction(signature, {
        commitment: "confirmed",
        maxSupportedTransactionVersion: 0,
      });
      if (!tx) await new Promise((r) => setTimeout(r, 250));
    }
    assert.isNotNull(tx, "transaction not found");
    const keys = tx!.transaction.message.getAccountKeys();
    const tag = Buffer.from("e445a52e51cb9a1d", "hex"); // EVENT_IX_TAG_LE
    const events: { name: string; data: any }[] = [];
    for (const inner of tx!.meta!.innerInstructions ?? []) {
      for (const ix of inner.instructions) {
        if (!keys.get(ix.programIdIndex)!.equals(program.programId)) continue;
        const data = Buffer.from(anchor.utils.bytes.bs58.decode(ix.data));
        if (!data.subarray(0, 8).equals(tag)) continue;
        const event = program.coder.events.decode(
          data.subarray(8).toString("base64")
        );
        if (event) events.push(event);
      }
    }
    return events;
  };
  const eventNames = async (signature: string) =>
    (await cpiEvents(signature)).map((e) => e.name);

  const expectError = async (p: Promise<any>, name: string) => {
    try {
      await p;
    } catch (e: any) {
      // simulation errors keep the program error in their logs, not in their message
      const text = [
        String(e),
        e?.error?.errorCode?.code,
        ...(e?.logs ?? []),
        ...(e?.simulationResponse?.logs ?? []),
        ...(e?.transactionLogs ?? []),
      ].join("\n");
      assert.include(text, name);
      return;
    }
    assert.fail(`expected ${name}`);
  };

  before(async () => {
    [feedAdmin] = await newSigners(connection, 1);
    forwarder = new Forwarder(forwarderProgram, provider)
      .withState(Keypair.generate())
      .withOracles(1, 21, 1);
    await forwarder.initialize();
    await forwarder.initOraclesConfig();
    otherForwarder = new Forwarder(forwarderProgram, provider)
      .withState(Keypair.generate())
      .withOracles(1, 22, 1);
    await otherForwarder.initialize();
    await otherForwarder.initOraclesConfig();
    forwarderAuthority = calculateForwarderAuthorityBump(
      forwarder.state.publicKey,
      program.programId,
      forwarderProgram.programId
    )[0];

    const slot = await connection.getSlot();
    baseTimestamp = (await connection.getBlockTime(slot))! - 600;
  });
  beforeEach(initStore);

  describe("configuration", function () {
    it("initializes", async () => {
      const s = await program.account.storeState.fetch(state.publicKey);
      assert.isTrue(s.owner.equals(provider.publicKey));
      assert.equal(s.defaultMaxStoredReports.toNumber(), 4);
      assert.equal(s.maxObservationForwardSecs, 3600);
      assert.isTrue(s.df1Program.equals(df1.programId));
      assert.equal(s.feedAdmins.len.toNumber(), 1);
      assert.equal(s.defaultWorkflowMetadata.len.toNumber(), 1);
    });

    it("lets only the upgrade authority create stores", async () => {
      const other = Keypair.generate();
      await expectError(
        initializeStore(feedAdmin.keypair.publicKey, other.publicKey)
          .preInstructions([
            await createAccount(other, program.account.storeState.size, program.programId),
          ])
          .signers([other, feedAdmin.keypair])
          .rpc(),
        "Unauthorized"
      );
      await expectError(
        initializeStore(provider.publicKey, state.publicKey).rpc(),
        "ConstraintZero"
      );
    });

    it("gives every error its own code", () => {
      const codes = program.idl.errors!.map((e) => e.code);
      assert.equal(new Set(codes).size, codes.length);
    });

    it("rejects non feed admins, invalid data ids and other feeds' accounts", async () => {
      const id = newDataId();
      await expectError(
        program.methods
          .setFeeds([Array.from(id)], [""], [])
          .accountsStrict(adminAccounts(provider.publicKey))
          .remainingAccounts([meta(feedPda(id))])
          .rpc(),
        "Unauthorized"
      );
      const zero = Buffer.alloc(16);
      await expectError(setFeeds([zero]), "InvalidDataId");
      await expectError(setFeeds([withSvrPath(id, 1)]), "InvalidDataId");
      await expectError(
        program.methods
          .setFeeds([Array.from(id)], [""], [])
          .accountsStrict(adminAccounts())
          .remainingAccounts([meta(feedPda(newDataId()))])
          .signers([feedAdmin.keypair])
          .rpc(),
        "AccountMismatch"
      );
    });

    it("sets feeds: creates them, with descriptions and their own workflows", async () => {
      const [a, b] = [newDataId(), newDataId()];
      const before = await connection.getBalance(feedAdmin.keypair.publicKey);
      const signature = await setFeeds([a, b], ["ETH / USD", "BTC / USD"], [workflow()]);
      assert.isBelow(
        await connection.getBalance(feedAdmin.keypair.publicKey),
        before,
        "the feed admin pays the rent"
      );
      assert.includeMembers(await eventNames(signature), [
        "feedsCreated",
        "feedDescriptionSet",
        "specificFeedMetadataSet",
      ]);
      const description = (id: Buffer) =>
        program.methods.description(Array.from(id)).accountsStrict(read(id)).view();
      assert.equal(await description(a), "ETH / USD");
      assert.equal(await description(b), "BTC / USD");
      const feed = await program.account.feed.fetch(feedPda(a));
      assert.equal(feed.maxStoredReports.toNumber(), 4, "the default ring");
      assert.equal(feed.minDecimals, 18, "strictest minimum by default");
      assert.equal(feed.specificWorkflowMetadata.len.toNumber(), 1);

      // setting an existing feed again keeps its rounds and changes its description
      await report([{ dataId: a, timestamp: nextTimestamp(), answer: new BN(1) }]);
      await setFeeds([a], ["ETH / USD (1)"]);
      assert.equal(await description(a), "ETH / USD (1)");
      assert.equal((await readLatest(a)).roundId.toNumber(), 1);
      await expectError(setFeeds([a], ["ETH\u0000USD"]), "InvalidDescription");
    });

    it("checks feed permissions", async () => {
      const id = newDataId();
      const check = (feed: PublicKey | null) =>
        program.methods
          .checkFeedPermission(Array.from(id), workflow())
          .accountsStrict({ state: state.publicKey, feed })
          .view();
      assert.isTrue(await check(null), "default metadata");
      await setFeeds([id], [""], [workflow(randomBytes(10))]);
      assert.isFalse(
        await check(feedPda(id)),
        "specific metadata replaces the default"
      );
      await program.methods
        .removeSpecificFeedMetadata([Array.from(id)])
        .accountsStrict(adminAccounts())
        .remainingAccounts([meta(feedPda(id))])
        .signers([feedAdmin.keypair])
        .rpc();
      assert.isTrue(await check(feedPda(id)), "back to the default");
    });
  });

  describe("reports", function () {
    it("writes feeds that are set, skips the rest", async () => {
      const [set, unset] = [newDataId(), newDataId()];
      await setFeeds([set]);
      const ts = nextTimestamp();
      const signature = await report([
        { dataId: set, timestamp: ts, answer: new BN(1234).mul(E18).divn(100) },
        { dataId: unset, timestamp: ts, answer: new BN(1) },
      ]);
      const events = await cpiEvents(signature);
      assert.includeMembers(events.map((e) => e.name), [
        "feedUpdated",
        "feedNotInitializedIgnored",
      ]);
      // skip events before the permission check carry the sender: anyone can cause them
      for (const e of events.filter((e) => e.name.endsWith("Ignored"))) {
        assert.isTrue(e.data.sender.equals(forwarderAuthority));
      }
      assert.isFalse(await exists(feedPda(unset)), "reports never create feeds");

      const round = await readLatest(set);
      assert.equal(round.roundId.toNumber(), 1);
      assert.equal(round.observationsTimestamp, ts);
      assert.isTrue(round.answer.eq(new BN(1234).mul(E18).divn(100)));
    });

    it("writes only with a permitted workflow", async () => {
      const id = newDataId();
      await setFeeds([id]);
      const signature = await report(
        [{ dataId: id, timestamp: nextTimestamp(), answer: new BN(1) }],
        { via: otherForwarder }
      );
      assert.include(await eventNames(signature), "invalidUpdatePermission");
      await expectError(readLatest(id), "NoDataPresent");
    });

    it("refuses another feed's account", async () => {
      const [id, other] = [newDataId(), newDataId()];
      await setFeeds([id, other]);
      await expectError(
        report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(2) }], {
          accounts: () => [meta(feedPda(other))],
        }),
        "AccountMismatch"
      );
    });

    it("skips invalid, future, stale and out of range reports", async () => {
      const [a, b] = [newDataId(), newDataId()];
      await setFeeds([a, b]);
      const ts = nextTimestamp();
      await report([{ dataId: a, timestamp: ts, answer: new BN(1) }]);
      await program.methods
        .setMinMaxAnswers([Array.from(b)], [new BN(-10)], [new BN(10)])
        .accountsStrict(adminAccounts())
        .remainingAccounts([meta(feedPda(b))])
        .signers([feedAdmin.keypair])
        .rpc();

      // two transactions: a legacy transaction fits fewer reports than the program takes
      const first = await report([
        { dataId: withSvrPath(newDataId(), 1), timestamp: nextTimestamp(), answer: new BN(1) },
        { dataId: a, timestamp: ts, answer: new BN(2) },
        { dataId: a, timestamp: baseTimestamp + 1_000_000, answer: new BN(3) },
      ]);
      const second = await report([
        { dataId: b, timestamp: nextTimestamp(), answer: new BN(11) },
        { dataId: b, timestamp: nextTimestamp(), answer: new BN(9) },
      ]);
      assert.includeMembers([...(await eventNames(first)), ...(await eventNames(second))], [
        "invalidDataIdIgnored",
        "staleReport",
        "futureReportIgnored",
        "answerOutOfMinMaxRange",
        "feedUpdated",
      ]);
      assert.isTrue((await latestAnswer(a)).eq(new BN(1)), "a untouched");
      assert.isTrue((await latestAnswer(b)).eq(new BN(9)), "in range lands");
    });

    it("applies several reports of one feed in a batch in order", async () => {
      const id = newDataId();
      await setFeeds([id]);
      const [t1, t2] = [nextTimestamp(), nextTimestamp()];
      const signature = await report([
        { dataId: id, timestamp: t1, answer: new BN(1) },
        { dataId: id, timestamp: t2, answer: new BN(2) },
        { dataId: id, timestamp: t1, answer: new BN(3) },
      ]);
      const events = await cpiEvents(signature);
      assert.deepEqual(
        events.filter((e) => e.name === "feedUpdated").map((e) => e.data.roundId.toNumber()),
        [1, 2]
      );
      assert.include(events.map((e) => e.name), "staleReport");
      const latest = await readLatest(id);
      assert.equal(latest.roundId.toNumber(), 2);
      assert.isTrue(latest.answer.eq(new BN(2)));
    });

    it("disables writes for every workflow, reads continue", async () => {
      const id = newDataId();
      await setFeeds([id]);
      await report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(1) }]);
      const disable = (disabled: boolean) =>
        program.methods
          .setFeedWritesDisabled([Array.from(id)], disabled)
          .accountsStrict(adminAccounts())
          .remainingAccounts([meta(feedPda(id))])
          .signers([feedAdmin.keypair])
          .rpc();
      await disable(true);
      let signature = await report([
        { dataId: id, timestamp: nextTimestamp(), answer: new BN(2) },
      ]);
      assert.include(await eventNames(signature), "feedWritesDisabledIgnored");
      assert.isTrue((await latestAnswer(id)).eq(new BN(1)), "still readable");

      // also with feed-specific workflows
      await setFeeds([id], [""], [workflow()]);
      signature = await report([
        { dataId: id, timestamp: nextTimestamp(), answer: new BN(2) },
      ]);
      assert.include(await eventNames(signature), "feedWritesDisabledIgnored");

      await disable(false);
      await report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(3) }]);
      assert.isTrue((await latestAnswer(id)).eq(new BN(3)));
    });

    it("freezes reads, writes continue", async () => {
      const id = newDataId();
      await setFeeds([id]);
      await report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(1) }]);
      const freeze = (frozen: boolean) =>
        program.methods
          .setFeedFrozen([Array.from(id)], frozen)
          .accountsStrict(adminAccounts())
          .remainingAccounts([meta(feedPda(id))])
          .signers([feedAdmin.keypair])
          .rpc();
      await freeze(true);
      await expectError(readLatest(id), "FeedFrozen");
      await report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(2) }]);
      await freeze(false);
      assert.equal((await readLatest(id)).roundId.toNumber(), 2);
    });

    it("grows a feed's history, before its ring wraps", async () => {
      const id = newDataId();
      await setFeeds([id]);
      await program.methods
        .setMaxStoredReports([Array.from(id)], [new BN(6)])
        .accountsStrict(adminAccounts())
        .remainingAccounts([meta(feedPda(id))])
        .signers([feedAdmin.keypair])
        .rpc();
      const feed = await program.account.feed.fetch(feedPda(id));
      assert.equal(feed.maxStoredReports.toNumber(), 6);
    });

    it("fails a report whose feed is passed read-only", async () => {
      const id = newDataId();
      await setFeeds([id]);
      await expectError(
        report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(2) }], {
          accounts: (r) => [meta(feedPda(r.dataId), false)],
        }),
        "AccountNotWritable"
      );
    });

    it("reads at the caller's decimals, down to the feed's minimum", async () => {
      const id = newDataId();
      await setFeeds([id]);
      await report([
        { dataId: id, timestamp: nextTimestamp(), answer: new BN(123456789).mul(E18).divn(100) },
      ]);
      await expectError(readLatest(id, 8), "InvalidDecimals");
      await expectError(readLatest(id, 19), "InvalidDecimals");
      await program.methods
        .setMinDecimals([Array.from(id)], 0)
        .accountsStrict(adminAccounts())
        .remainingAccounts([meta(feedPda(id))])
        .signers([feedAdmin.keypair])
        .rpc();
      assert.equal(
        await program.methods.getMinDecimals(Array.from(id)).accountsStrict(read(id)).view(),
        0
      );
      assert.equal(
        await program.methods.decimals(Array.from(id)).accountsStrict(read(id)).view(),
        18
      );
      assert.isTrue(
        (await readLatest(id, 8)).answer.eq(new BN(123456789).mul(E8).divn(100)),
        "rescaled"
      );
      const round = await program.methods
        .getRound(Array.from(id), new BN(1), 0)
        .accountsStrict(read(id))
        .view();
      assert.isTrue(round.answer.eq(new BN(1234567)), "truncated");
      await report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(1) }]);
      await expectError(readLatest(id, 8), "RoundsToZero");
    });

    it("keeps history in a ring", async () => {
      const id = newDataId();
      await setFeeds([id]);
      for (let i = 1; i <= 5; i++) {
        await report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(i) }]);
      }
      const round = (r: number) =>
        program.methods.getRound(Array.from(id), new BN(r), 18).accountsStrict(read(id)).view();
      // a ring of 4: rounds 2..=5 kept
      assert.isTrue((await round(2)).answer.eq(new BN(2)));
      assert.isTrue((await round(5)).answer.eq(new BN(5)));
      await expectError(round(1), "NoDataPresent");
      await expectError(round(6), "NoDataPresent");
    });
  });

  describe("DF1 write-through", function () {
    // DF1 feed with a single-slot live buffer
    const createDf1Feed = async (decimals: number) => {
      const feed = Keypair.generate();
      await df1.methods
        .createFeed("df1 feed", decimals, 1, 1)
        .accounts({ feed: feed.publicKey, authority: provider.publicKey })
        .signers([feed])
        .preInstructions([
          await df1.account.transmissions.createInstruction(feed, 8 + 192 + 48),
        ])
        .rpc();
      return feed.publicKey;
    };
    const setWriter = (feed: PublicKey, writer: PublicKey) =>
      df1.methods
        .setWriter(writer)
        .accounts({ feed, owner: provider.publicKey, authority: provider.publicKey })
        .rpc();
    const df1Round = async (feed: PublicKey) => {
      const header = await df1.account.transmissions.fetch(feed);
      const live = (await connection.getAccountInfo(feed))!.data.subarray(200, 248);
      return {
        roundId: header.latestRoundId,
        timestamp: live.readUInt32LE(8),
        answer: new BN(live.subarray(16, 32), "le").fromTwos(128),
      };
    };
    const link = (id: Buffer, feed: PublicKey, disabled = false) =>
      program.methods
        .setDf1Feeds([Array.from(id)], [feed], [disabled])
        .accountsStrict(adminAccounts())
        .remainingAccounts(
          feed.equals(PublicKey.default)
            ? [meta(feedPda(id))]
            : [meta(feedPda(id)), meta(feed, false)]
        )
        .signers([feedAdmin.keypair])
        .rpc();
    const skipReason = async (signature: string) =>
      (await cpiEvents(signature)).find((e) => e.name === "df1WriteSkipped")?.data
        .reason;

    it("keeps a linked DF1 feed written once it is the writer", async () => {
      const id = newDataId();
      const legacy = await createDf1Feed(8);
      // linking creates the feed too
      await link(id, legacy);
      const df1Feeds = new Map([[id.toString("hex"), legacy]]);

      // writer still OCR2: the round is stored, DF1 is skipped
      let signature = await report(
        [{ dataId: id, timestamp: nextTimestamp(), answer: new BN(2).mul(E18) }],
        { df1Feeds }
      );
      assert.deepEqual(await skipReason(signature), { notWriter: {} });
      assert.isTrue((await latestAnswer(id)).eq(new BN(2).mul(E18)));
      assert.equal((await df1Round(legacy)).roundId, 0);

      // cutover
      await setWriter(legacy, df1Writer());
      const ts = nextTimestamp();
      await report(
        [{ dataId: id, timestamp: ts, answer: new BN(105).mul(E18).divn(100) }],
        { df1Feeds }
      );
      let round = await df1Round(legacy);
      assert.equal(round.roundId, 1);
      assert.equal(round.timestamp, ts);
      assert.isTrue(round.answer.eq(new BN(105).mul(E8).divn(100)), "rescaled");

      // not passed: DF1 skipped, the round still stored
      signature = await report(
        [{ dataId: id, timestamp: nextTimestamp(), answer: new BN(3).mul(E18) }],
        { df1Feeds, passDf1: false }
      );
      assert.deepEqual(await skipReason(signature), { notPassed: {} });
      assert.isTrue((await latestAnswer(id)).eq(new BN(3).mul(E18)));

      // DF1 already has a newer round
      await setWriter(legacy, provider.publicKey);
      await df1.methods
        .submit({ timestamp: new BN(baseTimestamp + 100_000), answer: new BN(1) })
        .accounts({ feed: legacy, authority: provider.publicKey })
        .rpc();
      await setWriter(legacy, df1Writer());
      signature = await report(
        [{ dataId: id, timestamp: nextTimestamp(), answer: new BN(4).mul(E18) }],
        { df1Feeds }
      );
      assert.deepEqual(await skipReason(signature), { stale: {} });

      // disabled
      await link(id, legacy, true);
      signature = await report(
        [{ dataId: id, timestamp: nextTimestamp(), answer: new BN(5).mul(E18) }],
        { df1Feeds }
      );
      assert.deepEqual(await skipReason(signature), { disabled: {} });
      round = await df1Round(legacy);
      assert.equal(round.roundId, 2, "only the direct submit since");

      // rollback is DF1's set_writer; unlinking drops the account from reports
      await link(id, PublicKey.default);
      await report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(6).mul(E18) }]);
      assert.isTrue((await latestAnswer(id)).eq(new BN(6).mul(E18)));
    });

    it("requires the linked DF1 feed in reports", async () => {
      const id = newDataId();
      const legacy = await createDf1Feed(8);
      await link(id, legacy);
      await expectError(
        report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(1) }]),
        "InvalidAccountCount"
      );
    });

    it("only links DF1 feeds submit can write", async () => {
      const id = newDataId();
      await expectError(link(id, state.publicKey), "InvalidDf1Feed");
    });
  });
});
