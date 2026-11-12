import * as anchor from "@coral-xyz/anchor";
import { Program, BN } from "@coral-xyz/anchor";
import {
  AccountMeta,
  Keypair,
  LAMPORTS_PER_SOL,
  PublicKey,
  SystemProgram,
} from "@solana/web3.js";
import { assert } from "chai";
import { randomBytes } from "crypto";
import { DataFeedsStore } from "../target/types/data_feeds_store";
import { DataFeedsProxy } from "../target/types/data_feeds_proxy";
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
  const proxy = anchor.workspace.DataFeedsProxy as Program<DataFeedsProxy>;
  const forwarderProgram = anchor.workspace
    .KeystoneForwarder as Program<KeystoneForwarder>;
  const df1 = anchor.workspace.Store as Program<Store>;

  let feedAdmin: Signer;
  let forwarder: Forwarder;
  let otherForwarder: Forwarder;
  let forwarderAuthority: PublicKey;
  let state: Keypair;
  let proxyState: Keypair;
  const workflowOwner = randomBytes(20);
  const workflowName = randomBytes(10);

  const storePda = (seed: string, ...keys: Buffer[]) =>
    PublicKey.findProgramAddressSync(
      [Buffer.from(seed), state.publicKey.toBuffer(), ...keys],
      program.programId
    )[0];
  const feedConfig = (id: Buffer) => storePda("feed_config", seed32(id));
  const vault = () => storePda("store_authority");
  const feedWriter = (s = state.publicKey) =>
    PublicKey.findProgramAddressSync(
      [Buffer.from("feed_writer"), s.toBuffer()],
      program.programId
    )[0];
  const df1Writer = () => storePda("df1_writer");
  const proxyFeed = (id: Buffer) =>
    PublicKey.findProgramAddressSync(
      [Buffer.from("feed"), proxyState.publicKey.toBuffer(), seed32(id)],
      proxy.programId
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
        proxyProgram: proxy.programId,
        proxyState: proxyState.publicKey,
        df1Program: df1.programId,
      })
      .accountsStrict({
        owner,
        state: account,
        programData: programData(program.programId),
        ...eventAccounts(),
      });
  const fundVault = async (lamports = LAMPORTS_PER_SOL) =>
    provider.sendAndConfirm(
      new anchor.web3.Transaction().add(
        SystemProgram.transfer({
          fromPubkey: provider.publicKey,
          toPubkey: vault(),
          lamports,
        })
      )
    );

  // a proxy trusting a new store, with a funded vault and the forwarder as default workflow
  const initStore = async () => {
    state = Keypair.generate();
    proxyState = Keypair.generate();
    await proxy.methods
      .initialize(feedWriter())
      .accountsStrict({
        owner: provider.publicKey,
        state: proxyState.publicKey,
        programData: programData(proxy.programId),
        ...eventAccounts(proxy.programId),
      })
      .preInstructions([
        await createAccount(
          proxyState,
          proxy.account.proxyState.size,
          proxy.programId
        ),
      ])
      .signers([proxyState])
      .rpc();
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
    await fundVault();
    await program.methods
      .setDefaultFeedMetadata([workflow()])
      .accountsStrict({
        feedAdmin: feedAdmin.keypair.publicKey,
        state: state.publicKey,
        ...eventAccounts(),
      })
      .signers([feedAdmin.keypair])
      .rpc();
  };
  const workflow = (name = workflowName) => ({
    allowedSender: forwarderAuthority,
    allowedWorkflowOwner: Array.from(workflowOwner),
    allowedWorkflowName: Array.from(name),
  });

  const configAccounts = (admin = feedAdmin.keypair.publicKey) => ({
    feedAdmin: admin,
    state: state.publicKey,
    systemProgram: SystemProgram.programId,
    ...eventAccounts(),
  });
  const proxyAccounts = (admin = feedAdmin.keypair.publicKey) => ({
    feedAdmin: admin,
    state: state.publicKey,
    proxyProgram: proxy.programId,
    proxyState: proxyState.publicKey,
    feedWriter: feedWriter(),
    systemProgram: SystemProgram.programId,
    ...eventAccounts(),
  });

  // Fixed accounts after the forwarder's, then per report: the proxy feed, the feed config and the
  // linked DF1 feed. Optional accounts (DF1 when a feed is linked, the vault when one may be
  // created) not passed are the program id. Legacy transactions keep test batches small.
  type ReportOpts = {
    df1Feeds?: Map<string, PublicKey>;
    passDf1?: boolean;
    passVault?: boolean;
    via?: Forwarder;
    accounts?: (r: Report) => AccountMeta[];
  };
  const reportAccounts = (reports: Report[], opts: ReportOpts = {}) => {
    const none = meta(program.programId, false);
    const passDf1 = opts.passDf1 ?? opts.df1Feeds !== undefined;
    const passVault = opts.passVault ?? true;
    const accounts: AccountMeta[] = [
      meta(state.publicKey, false),
      meta(proxy.programId, false),
      meta(proxyState.publicKey, false),
      meta(feedWriter(), false),
      passDf1 ? meta(df1.programId, false) : none,
      passDf1 ? meta(df1Writer(), false) : none,
      passVault ? meta(vault()) : none,
      passVault ? meta(SystemProgram.programId, false) : none,
      meta(eventAuthority(program.programId), false),
      meta(program.programId, false),
    ];
    for (const r of reports) {
      if (opts.accounts) {
        accounts.push(...opts.accounts(r));
        continue;
      }
      accounts.push(meta(proxyFeed(r.dataId)), meta(feedConfig(r.dataId)));
      const linked = opts.df1Feeds?.get(r.dataId.toString("hex"));
      if (linked) accounts.push(meta(linked));
    }
    return accounts;
  };
  const report = async (reports: Report[], opts: ReportOpts = {}) => {
    const { result } = await (opts.via ?? forwarder).report(
      program.programId,
      encodeReports(reports),
      workflowName,
      workflowOwner,
      reportAccounts(reports, opts)
    );
    return result;
  };

  // reads through the proxy
  const readLatest = async (id: Buffer, decimals = 18) =>
    (await proxy.methods
      .latestRound(Array.from(id), decimals)
      .accountsStrict({ state: proxyState.publicKey, feed: proxyFeed(id) })
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
      assert.isTrue(s.proxyProgram.equals(proxy.programId));
      assert.isTrue(s.proxyState.equals(proxyState.publicKey));
      assert.isTrue(s.df1Program.equals(df1.programId));
      assert.equal(s.feedAdmins.len.toNumber(), 1);
      assert.equal(s.defaultWorkflowMetadata.len.toNumber(), 1);
    });

    it("lets only the upgrade authority create stores", async () => {
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

    it("rejects non feed admins and invalid data ids", async () => {
      const id = newDataId();
      await expectError(
        program.methods
          .setFeedDescriptions([Array.from(id)], ["ETH / USD"])
          .accountsStrict(proxyAccounts(provider.publicKey))
          .remainingAccounts([meta(proxyFeed(id))])
          .rpc(),
        "Unauthorized"
      );
      await expectError(
        program.methods
          .setFeedWritesDisabled([Array.from(Buffer.alloc(16))], true)
          .accountsStrict(configAccounts())
          .remainingAccounts([meta(feedConfig(Buffer.alloc(16)))])
          .signers([feedAdmin.keypair])
          .rpc(),
        "InvalidDataId"
      );
    });

    it("sets descriptions through the proxy, creating the feed", async () => {
      const id = newDataId();
      const describe = (dataId: Buffer, text: string, feed = proxyFeed(dataId)) =>
        program.methods
          .setFeedDescriptions([Array.from(dataId)], [text])
          .accountsStrict(proxyAccounts())
          .remainingAccounts([meta(feed)])
          .signers([feedAdmin.keypair])
          .rpc();
      const signature = await describe(id, "ETH / USD");
      assert.includeMembers(await eventNames(signature), [
        "feedsCreated",
        "feedDescriptionSet",
      ]);
      assert.equal(
        await proxy.methods
          .description(Array.from(id))
          .accountsStrict({ state: proxyState.publicKey, feed: proxyFeed(id) })
          .view(),
        "ETH / USD"
      );
      await describe(id, "");
      await expectError(describe(id, "ETH\u0000USD"), "InvalidDescription");
      // the feed's first report creates the config the description call didn't
      assert.isFalse(await exists(feedConfig(id)));
      await report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(1) }]);
      assert.isTrue(await exists(feedConfig(id)));
      assert.equal((await readLatest(id)).roundId.toNumber(), 1);
      await expectError(
        describe(id, "x", proxyFeed(newDataId())),
        "AccountMismatch"
      );
    });

    it("checks feed permissions", async () => {
      const id = newDataId();
      const check = (config: PublicKey | null) =>
        program.methods
          .checkFeedPermission(Array.from(id), workflow())
          .accountsStrict({ state: state.publicKey, feedConfig: config })
          .view();
      assert.isTrue(await check(null), "default metadata");
      await program.methods
        .setSpecificFeedMetadata([Array.from(id)], [workflow(randomBytes(10))])
        .accountsStrict(configAccounts())
        .remainingAccounts([meta(feedConfig(id))])
        .signers([feedAdmin.keypair])
        .rpc();
      assert.isFalse(
        await check(feedConfig(id)),
        "specific metadata replaces the default"
      );
      assert.isFalse(await exists(proxyFeed(id)), "only the config was made");
    });
  });

  describe("reports", function () {
    it("creates a feed on its first report, paid by the vault", async () => {
      const id = newDataId();
      const before = await connection.getBalance(vault());
      const ts = nextTimestamp();
      const signature = await report([
        { dataId: id, timestamp: ts, answer: new BN(1234).mul(E18).divn(100) },
      ]);
      assert.includeMembers(await eventNames(signature), [
        "feedsCreated",
        "feedUpdated",
      ]);
      assert.isBelow(await connection.getBalance(vault()), before);

      const round = await readLatest(id);
      assert.equal(round.roundId.toNumber(), 1);
      assert.equal(round.observationsTimestamp, ts);
      assert.isTrue(round.answer.eq(new BN(1234).mul(E18).divn(100)));
      const feed = await proxy.account.feed.fetch(proxyFeed(id));
      assert.equal(feed.maxStoredReports.toNumber(), 4, "the default ring");
      assert.isTrue(await exists(feedConfig(id)));

      await report([
        { dataId: id, timestamp: nextTimestamp(), answer: new BN(-5).mul(E18) },
      ]);
      assert.equal((await readLatest(id)).roundId.toNumber(), 2);
      assert.isTrue((await latestAnswer(id)).eq(new BN(-5).mul(E18)));
    });

    it("creates nothing for workflows that may not write", async () => {
      const id = newDataId();
      const signature = await report(
        [{ dataId: id, timestamp: nextTimestamp(), answer: new BN(1) }],
        { via: otherForwarder }
      );
      assert.include(await eventNames(signature), "invalidUpdatePermission");
      assert.isFalse(await exists(proxyFeed(id)));
      assert.isFalse(await exists(feedConfig(id)));
    });

    it("skips new feeds it can't pay for, the rest of the batch lands", async () => {
      const [existing, fresh] = [newDataId(), newDataId()];
      await report([{ dataId: existing, timestamp: nextTimestamp(), answer: new BN(1) }]);

      // vault not passed
      let signature = await report(
        [
          { dataId: fresh, timestamp: nextTimestamp(), answer: new BN(2) },
          { dataId: existing, timestamp: nextTimestamp(), answer: new BN(3) },
        ],
        { passVault: false }
      );
      let events = await cpiEvents(signature);
      const skipped = events.find((e) => e.name === "feedCreationSkipped");
      assert.deepEqual(skipped?.data.reason, { vaultNotPassed: {} });
      assert.isTrue((await latestAnswer(existing)).eq(new BN(3)));
      assert.isFalse(await exists(proxyFeed(fresh)));

      // vault emptied
      const balance = await connection.getBalance(vault());
      await program.methods
        .recoverNative(new BN(balance))
        .accountsStrict({
          owner: provider.publicKey,
          state: state.publicKey,
          storeAuthority: vault(),
          to: provider.publicKey,
          systemProgram: SystemProgram.programId,
          ...eventAccounts(),
        })
        .rpc();
      signature = await report([
        { dataId: fresh, timestamp: nextTimestamp(), answer: new BN(2) },
      ]);
      events = await cpiEvents(signature);
      assert.deepEqual(
        events.find((e) => e.name === "feedCreationSkipped")?.data.reason,
        { insufficientVaultBalance: {} }
      );
      assert.isFalse(await exists(proxyFeed(fresh)));
    });

    it("refuses a feed config left out or replaced", async () => {
      const [id, other] = [newDataId(), newDataId()];
      await report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(1) }]);
      await report([{ dataId: other, timestamp: nextTimestamp(), answer: new BN(1) }]);
      // an empty account where the data id's config is: it can't stand for "no config"
      await expectError(
        report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(2) }], {
          accounts: (r) => [meta(proxyFeed(r.dataId)), meta(Keypair.generate().publicKey)],
        }),
        "AccountMismatch"
      );
      // another data id's config
      await expectError(
        report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(2) }], {
          accounts: (r) => [meta(proxyFeed(r.dataId)), meta(feedConfig(other))],
        }),
        "AccountMismatch"
      );
      // another data id's proxy feed
      await expectError(
        report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(2) }], {
          accounts: (r) => [meta(proxyFeed(other)), meta(feedConfig(r.dataId))],
        }),
        "AccountMismatch"
      );
    });

    it("skips invalid, future, stale and out of range reports", async () => {
      const [a, b] = [newDataId(), newDataId()];
      const ts = nextTimestamp();
      await report([{ dataId: a, timestamp: ts, answer: new BN(1) }]);
      await program.methods
        .setMinMaxAnswers([Array.from(b)], [new BN(-10)], [new BN(10)])
        .accountsStrict(configAccounts())
        .remainingAccounts([meta(feedConfig(b))])
        .signers([feedAdmin.keypair])
        .rpc();

      const invalid = await report([
        { dataId: withSvrPath(newDataId(), 1), timestamp: nextTimestamp(), answer: new BN(1) },
      ]);
      const first = await report([
        { dataId: a, timestamp: ts, answer: new BN(2) },
        { dataId: a, timestamp: baseTimestamp + 1_000_000, answer: new BN(3) },
      ]);
      const second = await report([
        { dataId: b, timestamp: nextTimestamp(), answer: new BN(11) },
        { dataId: b, timestamp: nextTimestamp(), answer: new BN(9) },
      ]);
      const names = [
        ...(await eventNames(invalid)),
        ...(await eventNames(first)),
        ...(await eventNames(second)),
      ];
      assert.includeMembers(names, [
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
      const [t1, t2] = [nextTimestamp(), nextTimestamp()];
      const signature = await report([
        { dataId: id, timestamp: t1, answer: new BN(1) },
        { dataId: id, timestamp: t2, answer: new BN(2) },
        { dataId: id, timestamp: t1, answer: new BN(3) },
      ]);
      const events = await cpiEvents(signature);
      const updated = events.filter((e) => e.name === "feedUpdated");
      assert.deepEqual(
        updated.map((e) => e.data.roundId.toNumber()),
        [1, 2]
      );
      assert.include(events.map((e) => e.name), "staleReport");
      const latest = await readLatest(id);
      assert.equal(latest.roundId.toNumber(), 2);
      assert.isTrue(latest.answer.eq(new BN(2)));
    });

    it("disables writes for every workflow, reads continue", async () => {
      const id = newDataId();
      await report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(1) }]);
      const disable = (dataId: Buffer, disabled: boolean) =>
        program.methods
          .setFeedWritesDisabled([Array.from(dataId)], disabled)
          .accountsStrict(configAccounts())
          .remainingAccounts([meta(feedConfig(dataId))])
          .signers([feedAdmin.keypair])
          .rpc();
      await disable(id, true);
      let signature = await report([
        { dataId: id, timestamp: nextTimestamp(), answer: new BN(2) },
      ]);
      assert.include(await eventNames(signature), "feedWritesDisabledIgnored");
      assert.isTrue((await latestAnswer(id)).eq(new BN(1)), "still readable");

      // also with feed-specific workflows
      await program.methods
        .setSpecificFeedMetadata([Array.from(id)], [workflow()])
        .accountsStrict(configAccounts())
        .remainingAccounts([meta(feedConfig(id))])
        .signers([feedAdmin.keypair])
        .rpc();
      signature = await report([
        { dataId: id, timestamp: nextTimestamp(), answer: new BN(2) },
      ]);
      assert.include(await eventNames(signature), "feedWritesDisabledIgnored");

      await disable(id, false);
      await report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(3) }]);
      assert.isTrue((await latestAnswer(id)).eq(new BN(3)));

      // a data id disabled before its first report is never created
      const blocked = newDataId();
      await disable(blocked, true);
      await report([{ dataId: blocked, timestamp: nextTimestamp(), answer: new BN(1) }]);
      assert.isFalse(await exists(proxyFeed(blocked)));
    });

    it("freezes reads, writes continue", async () => {
      const id = newDataId();
      await report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(1) }]);
      const freeze = (frozen: boolean) =>
        program.methods
          .setFeedFrozen([Array.from(id)], frozen)
          .accountsStrict(proxyAccounts())
          .remainingAccounts([meta(proxyFeed(id))])
          .signers([feedAdmin.keypair])
          .rpc();
      await freeze(true);
      await expectError(readLatest(id), "FeedFrozen");
      await report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(2) }]);
      await freeze(false);
      assert.equal((await readLatest(id)).roundId.toNumber(), 2);
    });

    it("grows a feed's history through the proxy", async () => {
      const id = newDataId();
      await report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(1) }]);
      await program.methods
        .setMaxStoredReports([Array.from(id)], [new BN(6)])
        .accountsStrict(proxyAccounts())
        .remainingAccounts([meta(proxyFeed(id))])
        .signers([feedAdmin.keypair])
        .rpc();
      const feed = await proxy.account.feed.fetch(proxyFeed(id));
      assert.equal(feed.maxStoredReports.toNumber(), 6);
    });

    it("fails a report whose proxy feed is passed read-only", async () => {
      const id = newDataId();
      await report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(1) }]);
      await expectError(
        report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(2) }], {
          accounts: (r) => [meta(proxyFeed(r.dataId), false), meta(feedConfig(r.dataId))],
        }),
        "AccountNotWritable"
      );
    });

    it("hands over to a new store, round ids carry on", async () => {
      const id = newDataId();
      await report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(1) }]);
      const oldState = state;

      // a second store writing the same proxy
      state = Keypair.generate();
      await initializeStore(provider.publicKey, state.publicKey)
        .preInstructions([
          await createAccount(state, program.account.storeState.size, program.programId),
        ])
        .signers([state])
        .rpc();
      await fundVault();
      await program.methods
        .setDefaultFeedMetadata([workflow()])
        .accountsStrict({
          feedAdmin: feedAdmin.keypair.publicKey,
          state: state.publicKey,
          ...eventAccounts(),
        })
        .signers([feedAdmin.keypair])
        .rpc();
      await proxy.methods
        .setStore(feedWriter())
        .accountsStrict({
          owner: provider.publicKey,
          state: proxyState.publicKey,
          ...eventAccounts(proxy.programId),
        })
        .rpc();

      await report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(2) }]);
      assert.equal((await readLatest(id)).roundId.toNumber(), 2, "no restart");

      // the old store can no longer write
      const newState = state;
      state = oldState;
      await expectError(
        report([{ dataId: id, timestamp: nextTimestamp(), answer: new BN(3) }]),
        "Unauthorized"
      );
      state = newState;
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
        .accountsStrict(configAccounts())
        .remainingAccounts(
          feed.equals(PublicKey.default)
            ? [meta(feedConfig(id))]
            : [meta(feedConfig(id)), meta(feed, false)]
        )
        .signers([feedAdmin.keypair])
        .rpc();
    const skipReason = async (signature: string) =>
      (await cpiEvents(signature)).find((e) => e.name === "df1WriteSkipped")?.data
        .reason;

    it("keeps a linked DF1 feed written once it is the writer", async () => {
      const id = newDataId();
      const legacy = await createDf1Feed(8);
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
      signature = await report(
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
      await expectError(link(id, proxyState.publicKey), "InvalidDf1Feed");
    });
  });
});
