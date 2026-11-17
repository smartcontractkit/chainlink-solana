import * as anchor from "@coral-xyz/anchor";
import { Program, BN } from "@coral-xyz/anchor";
import {
  AccountMeta,
  Keypair,
  PublicKey,
  SystemProgram,
} from "@solana/web3.js";
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
  const eventAccounts = () => ({
    eventAuthority: eventAuthority(program.programId),
    program: program.programId,
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
  const setDefault = (workflows = [workflow()]) =>
    program.methods
      .setDefaultFeedMetadata(workflows)
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
  // a feed admin instruction, signed by the feed admin, with its remaining accounts
  const asFeedAdmin = (
    call: {
      accountsStrict(accounts: ReturnType<typeof adminAccounts>): {
        remainingAccounts(accounts: AccountMeta[]): {
          signers(signers: Keypair[]): { rpc(): Promise<string> };
        };
      };
    },
    remaining: AccountMeta[]
  ) =>
    call
      .accountsStrict(adminAccounts())
      .remainingAccounts(remaining)
      .signers([feedAdmin.keypair])
      .rpc();
  const setFeeds = (
    ids: Buffer[],
    descriptions = ids.map(() => ""),
    workflows: ReturnType<typeof workflow>[] = []
  ) =>
    asFeedAdmin(
      program.methods.setFeeds(
        ids.map((id) => Array.from(id)),
        descriptions,
        workflows
      ),
      ids.map((id) => meta(feedPda(id)))
    );

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

  // reads through the store, at the caller's decimals
  const read = (id: Buffer) => ({ state: state.publicKey, feed: feedPda(id) });
  const readLatest = async (id: Buffer, decimals = 18) =>
    (await program.methods
      .latestRound(Array.from(id), decimals)
      .accountsStrict(read(id))
      .view())!;
  const latestAnswer = async (id: Buffer) =>
    (await readLatest(id)).answer as BN;
  const getRound = async (id: Buffer, roundId: number, decimals = 18) =>
    (await program.methods
      .getRound(Array.from(id), new BN(roundId), decimals)
      .accountsStrict(read(id))
      .view())!;
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

    it("lets only the upgrade authority create stores, once", async () => {
      const other = Keypair.generate();
      await expectError(
        initializeStore(feedAdmin.keypair.publicKey, other.publicKey)
          .preInstructions([
            await createAccount(
              other,
              program.account.storeState.size,
              program.programId
            ),
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
        asFeedAdmin(program.methods.setFeeds([Array.from(id)], [""], []), [
          meta(feedPda(newDataId())),
        ]),
        "AccountMismatch"
      );
    });

    it("sets feeds: creates them, with descriptions and their own workflows", async () => {
      const [a, b] = [newDataId(), newDataId()];
      const before = await connection.getBalance(feedAdmin.keypair.publicKey);
      const signature = await setFeeds(
        [a, b],
        ["ETH / USD", "BTC / USD"],
        [workflow()]
      );
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
        program.methods
          .description(Array.from(id))
          .accountsStrict(read(id))
          .view();
      assert.equal(await description(a), "ETH / USD");
      assert.equal(await description(b), "BTC / USD");
      const feed = await program.account.feed.fetch(feedPda(a));
      assert.equal(feed.maxStoredReports.toNumber(), 4, "the default ring");
      assert.equal(feed.minDecimals, 18, "strictest minimum by default");
      assert.equal(feed.specificWorkflowMetadata.len.toNumber(), 1);

      // setting an existing feed again keeps its rounds and changes its description
      await report([
        { dataId: a, timestamp: nextTimestamp(), answer: new BN(1) },
      ]);
      await setFeeds([a], ["ETH / USD (1)"]);
      assert.equal(await description(a), "ETH / USD (1)");
      assert.equal((await readLatest(a)).roundId.toNumber(), 1);
      await expectError(setFeeds([a], ["ETH\u0000USD"]), "InvalidDescription");
    });

    it("checks feed permissions", async () => {
      const id = newDataId();
      const check = (feed: PublicKey) =>
        program.methods
          .checkFeedPermission(Array.from(id), workflow())
          .accountsStrict({ state: state.publicKey, feed })
          .view();
      assert.isFalse(await check(feedPda(id)), "a feed that isn't set");
      await expectError(check(feedPda(newDataId())), "AccountMismatch");
      await setFeeds([id]);
      assert.isTrue(await check(feedPda(id)), "default metadata");
      await setFeeds([id], [""], [workflow(randomBytes(10))]);
      assert.isFalse(
        await check(feedPda(id)),
        "specific metadata replaces the default"
      );
      await asFeedAdmin(
        program.methods.removeSpecificFeedMetadata([Array.from(id)]),
        [meta(feedPda(id))]
      );
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
      assert.includeMembers(
        events.map((e) => e.name),
        ["feedUpdated", "feedNotInitializedIgnored"]
      );
      // skip events before the permission check carry the sender: anyone can cause them
      for (const e of events.filter((e) => e.name.endsWith("Ignored"))) {
        assert.isTrue(e.data.sender.equals(forwarderAuthority));
      }
      assert.isFalse(
        await exists(feedPda(unset)),
        "reports never create feeds"
      );

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

    it("clears the default workflows, leaving feeds with their own", async () => {
      const [shared, own] = [newDataId(), newDataId()];
      await setFeeds([shared]);
      await setFeeds([own], [""], [workflow()]);
      await setDefault([]);
      try {
        const signature = await report([
          { dataId: shared, timestamp: nextTimestamp(), answer: new BN(1) },
          { dataId: own, timestamp: nextTimestamp(), answer: new BN(2) },
        ]);
        assert.deepEqual(await eventNames(signature), [
          "invalidUpdatePermission",
          "feedUpdated",
        ]);
        assert.isTrue((await latestAnswer(own)).eq(new BN(2)));
      } finally {
        await setDefault();
      }
    });

    it("refuses another feed's account", async () => {
      const [id, other] = [newDataId(), newDataId()];
      await setFeeds([id, other]);
      await expectError(
        report(
          [{ dataId: id, timestamp: nextTimestamp(), answer: new BN(2) }],
          {
            accounts: () => [meta(feedPda(other))],
          }
        ),
        "AccountMismatch"
      );
    });

    it("skips invalid, future, stale and out of range reports", async () => {
      const [a, b] = [newDataId(), newDataId()];
      await setFeeds([a, b]);
      const ts = nextTimestamp();
      await report([{ dataId: a, timestamp: ts, answer: new BN(1) }]);
      await asFeedAdmin(
        program.methods.setMinMaxAnswers(
          [Array.from(b)],
          [new BN(-10)],
          [new BN(10)]
        ),
        [meta(feedPda(b))]
      );

      // two transactions: a 1,232-byte transaction fits fewer reports than the program takes
      const first = await report([
        {
          dataId: withSvrPath(newDataId(), 1),
          timestamp: nextTimestamp(),
          answer: new BN(1),
        },
        { dataId: a, timestamp: ts, answer: new BN(2) },
        { dataId: a, timestamp: baseTimestamp + 1_000_000, answer: new BN(3) },
      ]);
      const second = await report([
        { dataId: b, timestamp: nextTimestamp(), answer: new BN(11) },
        { dataId: b, timestamp: nextTimestamp(), answer: new BN(9) },
      ]);
      assert.includeMembers(
        [...(await eventNames(first)), ...(await eventNames(second))],
        [
          "invalidDataIdIgnored",
          "staleReport",
          "futureReportIgnored",
          "answerOutOfMinMaxRange",
          "feedUpdated",
        ]
      );
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
        events
          .filter((e) => e.name === "feedUpdated")
          .map((e) => e.data.roundId.toNumber()),
        [1, 2]
      );
      assert.include(
        events.map((e) => e.name),
        "staleReport"
      );
      const latest = await readLatest(id);
      assert.equal(latest.roundId.toNumber(), 2);
      assert.isTrue(latest.answer.eq(new BN(2)));
    });

    it("disables writes for every workflow, reads continue", async () => {
      const id = newDataId();
      await setFeeds([id]);
      await report([
        { dataId: id, timestamp: nextTimestamp(), answer: new BN(1) },
      ]);
      const disable = (disabled: boolean) =>
        asFeedAdmin(
          program.methods.setFeedWritesDisabled([Array.from(id)], disabled),
          [meta(feedPda(id))]
        );
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
      await report([
        { dataId: id, timestamp: nextTimestamp(), answer: new BN(3) },
      ]);
      assert.isTrue((await latestAnswer(id)).eq(new BN(3)));
    });

    it("freezes reads, writes continue", async () => {
      const id = newDataId();
      await setFeeds([id]);
      await report([
        { dataId: id, timestamp: nextTimestamp(), answer: new BN(1) },
      ]);
      const freeze = (frozen: boolean) =>
        asFeedAdmin(program.methods.setFeedFrozen([Array.from(id)], frozen), [
          meta(feedPda(id)),
        ]);
      await freeze(true);
      await expectError(readLatest(id), "FeedFrozen");
      await report([
        { dataId: id, timestamp: nextTimestamp(), answer: new BN(2) },
      ]);
      await freeze(false);
      assert.equal((await readLatest(id)).roundId.toNumber(), 2);
    });

    it("grows a feed's history, before its ring wraps", async () => {
      const id = newDataId();
      await setFeeds([id]);
      const reportOne = (answer: number) =>
        report([
          { dataId: id, timestamp: nextTimestamp(), answer: new BN(answer) },
        ]);
      await reportOne(1);
      await reportOne(2);
      const grow = (ids: Buffer[], sizes: number[]) =>
        asFeedAdmin(
          program.methods.setMaxStoredReports(
            ids.map((i) => Array.from(i)),
            sizes.map((n) => new BN(n))
          ),
          ids.map((i) => meta(feedPda(i)))
        );
      await grow([id], [6]);
      const feed = await program.account.feed.fetch(feedPda(id));
      assert.equal(feed.maxStoredReports.toNumber(), 6);
      assert.isTrue(
        (await getRound(id, 1)).answer.eq(new BN(1)),
        "earlier rounds stay readable"
      );

      // at most 10 KiB per instruction, from the account's length when the call started: a feed
      // created in the call starts at 0, and a data ID listed twice grows from its first length
      await expectError(grow([newDataId()], [300]), "InvalidMaxStoredReports");
      await expectError(grow([id, id], [300, 600]), "InvalidMaxStoredReports");

      // a ring of 6 holding round 6 has wrapped
      for (let i = 3; i <= 6; i++) await reportOne(i);
      await expectError(grow([id], [8]), "MaxStoredReportsLocked");
    });

    it("fails a report whose feed is passed read-only", async () => {
      const id = newDataId();
      await setFeeds([id]);
      await expectError(
        report(
          [{ dataId: id, timestamp: nextTimestamp(), answer: new BN(2) }],
          {
            accounts: (r) => [meta(feedPda(r.dataId), false)],
          }
        ),
        "AccountNotWritable"
      );
    });

    it("reads at the caller's decimals, down to the feed's minimum", async () => {
      const id = newDataId();
      await setFeeds([id]);
      await report([
        {
          dataId: id,
          timestamp: nextTimestamp(),
          answer: new BN(123456789).mul(E18).divn(100),
        },
      ]);
      await expectError(readLatest(id, 8), "InvalidDecimals");
      await expectError(readLatest(id, 19), "InvalidDecimals");
      await asFeedAdmin(program.methods.setMinDecimals([Array.from(id)], 0), [
        meta(feedPda(id)),
      ]);
      assert.equal(
        await program.methods
          .getMinDecimals(Array.from(id))
          .accountsStrict(read(id))
          .view(),
        0
      );
      assert.equal(
        await program.methods
          .decimals(Array.from(id))
          .accountsStrict(read(id))
          .view(),
        18
      );
      assert.isTrue(
        (await readLatest(id, 8)).answer.eq(
          new BN(123456789).mul(E8).divn(100)
        ),
        "rescaled"
      );
      const round = await program.methods
        .getRound(Array.from(id), new BN(1), 0)
        .accountsStrict(read(id))
        .view();
      assert.isTrue(round.answer.eq(new BN(1234567)), "truncated");
      await report([
        { dataId: id, timestamp: nextTimestamp(), answer: new BN(1) },
      ]);
      await expectError(readLatest(id, 8), "RoundsToZero");
    });

    it("keeps history in a ring", async () => {
      const id = newDataId();
      await setFeeds([id]);
      for (let i = 1; i <= 5; i++) {
        await report([
          { dataId: id, timestamp: nextTimestamp(), answer: new BN(i) },
        ]);
      }
      const round = (r: number) =>
        program.methods
          .getRound(Array.from(id), new BN(r), 18)
          .accountsStrict(read(id))
          .view();
      // a ring of 4: rounds 2..=5 kept
      assert.isTrue((await round(2)).answer.eq(new BN(2)));
      assert.isTrue((await round(5)).answer.eq(new BN(5)));
      await expectError(round(1), "NoDataPresent");
      await expectError(round(6), "NoDataPresent");
    });
  });

  describe("random report batches", function () {
    // Seeded, so a failure replays: a model of the processing order (invalid ID, unset feed, write
    // freeze, permission, future, stale, range, then store) predicts every event and every feed.
    it("match a model of the report path", async () => {
      let seed = 20261117;
      const rand = () => {
        seed = (seed * 1103515245 + 12345) % 2147483648;
        return seed / 2147483648;
      };
      const pick = <T>(xs: T[]) => xs[Math.floor(rand() * xs.length)];

      const [open, freezable, bounded, own, unset] = [0, 1, 2, 3, 4].map(() =>
        newDataId()
      );
      const invalid = Buffer.alloc(16);
      await setFeeds([open, freezable, bounded]);
      // a feed whose own workflow list doesn't include the reporting workflow
      await setFeeds([own], [""], [workflow(randomBytes(10))]);
      await asFeedAdmin(
        program.methods.setMinMaxAnswers(
          [Array.from(bounded)],
          [new BN(0)],
          [new BN(1000)]
        ),
        [meta(feedPda(bounded))]
      );

      type Model = { roundId: number; timestamp: number; answer: number };
      const model = new Map<string, Model>(
        [open, freezable, bounded].map((id) => [
          id.toString("hex"),
          { roundId: 0, timestamp: 0, answer: 0 },
        ])
      );
      let writesDisabled = false;
      const expect = (id: Buffer, timestamp: number, answer: number) => {
        if (id.equals(invalid)) return "invalidDataIdIgnored";
        if (id.equals(unset)) return "feedNotInitializedIgnored";
        if (id.equals(freezable) && writesDisabled)
          return "feedWritesDisabledIgnored";
        if (id.equals(own)) return "invalidUpdatePermission";
        const feed = model.get(id.toString("hex"))!;
        if (timestamp > baseTimestamp + 600 + 3600)
          return "futureReportIgnored";
        if (timestamp <= feed.timestamp) return "staleReport";
        if (id.equals(bounded) && (answer < 0 || answer > 1000))
          return "answerOutOfMinMaxRange";
        feed.roundId += 1;
        feed.timestamp = timestamp;
        feed.answer = answer;
        return "feedUpdated";
      };

      for (let batch = 0; batch < 25; batch++) {
        if (rand() < 0.2) {
          writesDisabled = !writesDisabled;
          await asFeedAdmin(
            program.methods.setFeedWritesDisabled(
              [Array.from(freezable)],
              writesDisabled
            ),
            [meta(feedPda(freezable))]
          );
        }
        const reports: Report[] = [];
        const expected: string[] = [];
        for (let i = 1 + Math.floor(rand() * 3); i > 0; i--) {
          const id = pick([open, freezable, bounded, own, unset, invalid]);
          const latest = model.get(id.toString("hex"))?.timestamp ?? 0;
          const timestamp = pick([
            nextTimestamp(),
            nextTimestamp(),
            latest, // stale: equal to the latest
            Math.max(latest - 1, 0), // stale: older
            reports.length ? reports[reports.length - 1].timestamp : latest, // same as the last in the batch
            baseTimestamp + 1_000_000, // future
          ]);
          const answer = Math.floor(rand() * 1300) - 100;
          reports.push({ dataId: id, timestamp, answer: new BN(answer) });
          expected.push(expect(id, timestamp, answer));
        }
        const signature = await report(reports);
        assert.deepEqual(
          await eventNames(signature),
          expected,
          `batch ${batch}`
        );
        for (const [hex, want] of model) {
          const feed = await program.account.feed.fetch(
            feedPda(Buffer.from(hex, "hex"))
          );
          assert.equal(feed.roundId.toNumber(), want.roundId, `batch ${batch}`);
          assert.equal(feed.latestObservationsTimestamp, want.timestamp);
          assert.isTrue(feed.latestAnswer.eq(new BN(want.answer)));
        }
      }
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
        .accounts({
          feed,
          owner: provider.publicKey,
          authority: provider.publicKey,
        })
        .rpc();
    const df1Round = async (feed: PublicKey) => {
      const header = await df1.account.transmissions.fetch(feed);
      const live = (await connection.getAccountInfo(feed))!.data.subarray(
        200,
        248
      );
      return {
        roundId: header.latestRoundId,
        timestamp: live.readUInt32LE(8),
        answer: new BN(live.subarray(16, 32), "le").fromTwos(128),
      };
    };
    // marker of a linked DF1 feed, so no two data IDs link one DF1 feed
    const df1Link = (feed: PublicKey) =>
      PublicKey.findProgramAddressSync(
        [Buffer.from("df1_link"), state.publicKey.toBuffer(), feed.toBuffer()],
        program.programId
      )[0];
    const linkedDf1Feed = async (id: Buffer) =>
      (await exists(feedPda(id)))
        ? ((await program.account.feed.fetch(feedPda(id))).df1Feed as PublicKey)
        : PublicKey.default;
    const link = async (id: Buffer, feed: PublicKey, disabled = false) => {
      const current = await linkedDf1Feed(id);
      const accounts = [meta(feedPda(id))];
      if (!current.equals(PublicKey.default) && !current.equals(feed)) {
        accounts.push(meta(df1Link(current)));
      }
      if (!feed.equals(PublicKey.default)) {
        accounts.push(meta(feed, false), meta(df1Link(feed)));
      }
      return asFeedAdmin(
        program.methods.setDf1Feeds([Array.from(id)], [feed], [disabled]),
        accounts
      );
    };
    const skipReason = async (signature: string) =>
      (await cpiEvents(signature)).find((e) => e.name === "df1WriteSkipped")
        ?.data.reason;

    it("keeps a linked DF1 feed written once it is the writer", async () => {
      const id = newDataId();
      const legacy = await createDf1Feed(8);
      // linking creates the feed too
      await link(id, legacy);
      const df1Feeds = new Map([[id.toString("hex"), legacy]]);

      // writer still OCR2: the round is stored, DF1 is skipped
      let signature = await report(
        [
          {
            dataId: id,
            timestamp: nextTimestamp(),
            answer: new BN(2).mul(E18),
          },
        ],
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
        [
          {
            dataId: id,
            timestamp: nextTimestamp(),
            answer: new BN(3).mul(E18),
          },
        ],
        { df1Feeds, passDf1: false }
      );
      assert.deepEqual(await skipReason(signature), { notPassed: {} });
      assert.isTrue((await latestAnswer(id)).eq(new BN(3).mul(E18)));

      // DF1 already has a newer round
      await setWriter(legacy, provider.publicKey);
      await df1.methods
        .submit({
          timestamp: new BN(baseTimestamp + 100_000),
          answer: new BN(1),
        })
        .accounts({ feed: legacy, authority: provider.publicKey })
        .rpc();
      await setWriter(legacy, df1Writer());
      signature = await report(
        [
          {
            dataId: id,
            timestamp: nextTimestamp(),
            answer: new BN(4).mul(E18),
          },
        ],
        { df1Feeds }
      );
      assert.deepEqual(await skipReason(signature), { stale: {} });

      // disabled
      await link(id, legacy, true);
      signature = await report(
        [
          {
            dataId: id,
            timestamp: nextTimestamp(),
            answer: new BN(5).mul(E18),
          },
        ],
        { df1Feeds }
      );
      assert.deepEqual(await skipReason(signature), { disabled: {} });
      round = await df1Round(legacy);
      assert.equal(round.roundId, 2, "only the direct submit since");

      // rollback is DF1's set_writer; unlinking drops the account from reports
      await link(id, PublicKey.default);
      await report([
        { dataId: id, timestamp: nextTimestamp(), answer: new BN(6).mul(E18) },
      ]);
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

    it("fails a report whose DF1 feed is passed read-only", async () => {
      const id = newDataId();
      const legacy = await createDf1Feed(8);
      await link(id, legacy);
      await setWriter(legacy, df1Writer());
      await expectError(
        report(
          [{ dataId: id, timestamp: nextTimestamp(), answer: new BN(1) }],
          {
            df1Feeds: new Map([[id.toString("hex"), legacy]]),
            accounts: (r) => [meta(feedPda(r.dataId)), meta(legacy, false)],
          }
        ),
        "AccountNotWritable"
      );
    });

    it("links a DF1 feed from one data ID at a time", async () => {
      // the data ID a DF1 feed's marker holds, zero padded; all zero when unlinked
      const linkedFrom = async (feed: PublicKey) =>
        (await connection.getAccountInfo(df1Link(feed)))!.data.subarray(16, 48);
      const [a, b] = [newDataId(), newDataId()];
      const [x, y] = [await createDf1Feed(8), await createDf1Feed(8)];
      await link(a, x);
      assert.deepEqual(await linkedFrom(x), seed32(a));
      // relinking the same feed keeps its marker
      await link(a, x, true);
      await expectError(link(b, x), "Df1FeedAlreadyLinked");

      // moving a to y frees x for b
      await link(a, y);
      assert.deepEqual(await linkedFrom(x), Buffer.alloc(32));
      assert.deepEqual(await linkedFrom(y), seed32(a));
      await link(b, x);
      assert.deepEqual(await linkedFrom(x), seed32(b));

      // unlinking clears the marker
      await link(b, PublicKey.default);
      assert.deepEqual(await linkedFrom(x), Buffer.alloc(32));
      assert.isTrue((await linkedDf1Feed(b)).equals(PublicKey.default));
    });

    it("only links DF1 feeds submit can write", async () => {
      const id = newDataId();
      await expectError(link(id, state.publicKey), "InvalidDf1Feed");
    });
  });
});
