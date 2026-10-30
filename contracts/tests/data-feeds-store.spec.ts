import * as anchor from "@coral-xyz/anchor";
import { Program, BN } from "@coral-xyz/anchor";
import { AccountMeta, Keypair, PublicKey } from "@solana/web3.js";
import { assert } from "chai";
import { randomBytes } from "crypto";
import { DataFeedsStore } from "../target/types/data_feeds_store";
import { KeystoneForwarder } from "../target/types/keystone_forwarder";
import { Store } from "../target/types/store";
import { DummyReceiver } from "../target/types/dummy_receiver";
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
const withInstance = (id: Buffer, instance: number) => {
  const out = Buffer.from(id);
  out[11] = instance;
  return out;
};
// accounts key data ids zero padded to 32 bytes
const seed32 = (id: Buffer) => Buffer.concat([id, Buffer.alloc(16)]);
const indexedFeedId = (id: Buffer) => {
  const out = Buffer.from(id);
  out[11] = 0;
  out.fill(0, 12);
  return out;
};

describe("data feeds store", function () {
  this.timeout(60_000);
  const provider = anchor.AnchorProvider.env();
  anchor.setProvider(provider);
  const connection = provider.connection;

  const program = anchor.workspace.DataFeedsStore as Program<DataFeedsStore>;
  const forwarderProgram = anchor.workspace
    .KeystoneForwarder as Program<KeystoneForwarder>;
  const storeProgram = anchor.workspace.Store as Program<Store>;
  const dummyReceiver = anchor.workspace
    .DummyReceiver as Program<DummyReceiver>;

  let feedAdmin: Signer;
  let forwarder: Forwarder;
  let forwarderAuthority: PublicKey;
  let state: Keypair;
  const workflowOwner = randomBytes(20);
  const workflowName = randomBytes(10);

  const pda = (seed: string, ...keys: Buffer[]) =>
    PublicKey.findProgramAddressSync(
      [Buffer.from(seed), state.publicKey.toBuffer(), ...keys],
      program.programId
    )[0];
  const feedPda = (id: Buffer) => pda("feed", seed32(id));
  const callbackAuthority = (callbackProgram: PublicKey) =>
    pda("callback_authority", callbackProgram.toBuffer());
  const callbackProgramEntry = (callbackProgram: PublicKey) =>
    pda("callback_program", callbackProgram.toBuffer());
  // CPI events: #[event_cpi] contexts take the event authority PDA and the program
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

  const initStore = async (defaultMaxStoredReports = 8) => {
    state = Keypair.generate();
    await program.methods
      .initialize(
        [feedAdmin.keypair.publicKey],
        new BN(defaultMaxStoredReports),
        3600
      )
      .accountsStrict({
        owner: provider.publicKey,
        state: state.publicKey,
        systemProgram: anchor.web3.SystemProgram.programId,
        ...eventAccounts(),
      })
      .signers([state])
      .rpc();

    forwarderAuthority = calculateForwarderAuthorityBump(
      forwarder.state.publicKey,
      program.programId,
      forwarderProgram.programId
    )[0];

    await program.methods
      .setDefaultFeedMetadata([
        {
          allowedSender: forwarderAuthority,
          allowedWorkflowOwner: Array.from(workflowOwner),
          allowedWorkflowName: Array.from(workflowName),
        },
      ])
      .accountsStrict({
        feedAdmin: feedAdmin.keypair.publicKey,
        state: state.publicKey,
        ...eventAccounts(),
      })
      .signers([feedAdmin.keypair])
      .rpc();
  };

  const adminAccounts = () => ({
    feedAdmin: feedAdmin.keypair.publicKey,
    state: state.publicKey,
    systemProgram: anchor.web3.SystemProgram.programId,
    ...eventAccounts(),
  });

  // a feed account per data id, which also holds the data id's settings
  const setupFeed = async (...dataIds: Buffer[]) => {
    await program.methods
      .initFeeds(dataIds.map((id) => Array.from(id)))
      .accountsStrict(adminAccounts())
      .remainingAccounts(dataIds.map((id) => meta(feedPda(id))))
      .signers([feedAdmin.keypair])
      .rpc();
  };

  // per report: the feed of its data id, then callback program, its callback authority and the
  // configured accounts if a callback is configured
  const report = async (
    reports: Report[],
    callbackAccounts: Map<string, AccountMeta[]> = new Map(),
    via: Forwarder = forwarder
  ) => {
    const accounts: AccountMeta[] = [
      meta(state.publicKey, false),
      meta(eventAuthority(program.programId), false),
      meta(program.programId, false),
    ];
    for (const r of reports) {
      accounts.push(meta(feedPda(r.dataId)));
      accounts.push(...(callbackAccounts.get(r.dataId.toString("hex")) ?? []));
    }
    const { result } = await via.report(
      program.programId,
      encodeReports(reports),
      workflowName,
      workflowOwner,
      accounts
    );
    return result;
  };

  // the reader interface
  const readAccounts = (id: Buffer) => ({
    state: state.publicKey,
    feed: feedPda(id),
  });
  const readLatest = async (id: Buffer) =>
    (await program.methods
      .getLatestRound(Array.from(id))
      .accountsStrict(readAccounts(id))
      .view())!;
  const readRound = async (id: Buffer, round: number) =>
    (await program.methods
      .getRound(Array.from(id), new BN(round))
      .accountsStrict(readAccounts(id))
      .view())!;
  const latestAnswer = async (id: Buffer) =>
    (await readLatest(id)).answer as BN;

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

    const slot = await connection.getSlot();
    baseTimestamp = (await connection.getBlockTime(slot)) - 600;
  });

  describe("configuration", function () {
    beforeEach(async () => initStore());

    it("initializes", async () => {
      const s = await program.account.storeState.fetch(state.publicKey);
      assert.isTrue(s.owner.equals(provider.publicKey));
      assert.equal(s.defaultMaxStoredReports.toNumber(), 8);
      assert.equal(s.maxObservationForwardSecs, 3600);
      assert.equal(s.feedAdmins.len.toNumber(), 1);
      assert.equal(s.defaultWorkflowMetadata.len.toNumber(), 1);
    });

    it("gives every error its own code", () => {
      const errors = program.idl.errors!;
      const codes = errors.map((e) => e.code);
      assert.equal(new Set(codes).size, codes.length);
      assert.include(
        errors.map((e) => e.name),
        "unauthorized"
      );
    });

    it("rejects non feed admins", async () => {
      const id = newDataId();
      await expectError(
        program.methods
          .initFeeds([Array.from(id)])
          .accountsStrict({ ...adminAccounts(), feedAdmin: provider.publicKey })
          .remainingAccounts([meta(feedPda(id))])
          .rpc(),
        "Unauthorized"
      );
    });

    it("rejects the zero data id and reserved SVR path ids", async () => {
      const id = newDataId();
      for (const bad of [Buffer.alloc(16), withSvrPath(id, 1)]) {
        await expectError(
          program.methods
            .initFeeds([Array.from(bad)])
            .accountsStrict(adminAccounts())
            .remainingAccounts([meta(feedPda(bad))])
            .signers([feedAdmin.keypair])
            .rpc(),
          "InvalidDataId"
        );
      }
    });

    it("sets descriptions shared by every instance of a feed", async () => {
      const id = newDataId();
      const feedDescription = pda(
        "feed_description",
        seed32(indexedFeedId(id))
      );

      await program.methods
        .setFeedDescriptions([Array.from(indexedFeedId(id))], ["ETH/USD"])
        .accountsStrict(adminAccounts())
        .remainingAccounts([meta(feedDescription)])
        .signers([feedAdmin.keypair])
        .rpc();

      const description = (dataId: Buffer, account: PublicKey | null) =>
        program.methods
          .getDescription(Array.from(dataId))
          .accountsStrict({ state: state.publicKey, feedDescription: account })
          .view();
      assert.equal(await description(id, feedDescription), "ETH/USD");
      assert.equal(
        await description(withInstance(id, 3), feedDescription),
        "ETH/USD",
        "another instance of the feed"
      );
      assert.equal(await description(id, null), "", "no description account");
    });

    it("serves the stored decimals", async () => {
      assert.equal(
        await program.methods
          .getDecimals(Array.from(newDataId()))
          .accountsStrict({ state: state.publicKey, feedDescription: null })
          .view(),
        18
      );
    });

    it("checks feed permissions", async () => {
      const id = newDataId();
      await setupFeed(id);
      const workflow = {
        allowedSender: forwarderAuthority,
        allowedWorkflowOwner: Array.from(workflowOwner),
        allowedWorkflowName: Array.from(workflowName),
      };
      const check = () =>
        program.methods
          .checkFeedPermission(Array.from(id), workflow)
          .accountsStrict({ state: state.publicKey, feed: feedPda(id) })
          .view();

      assert.isTrue(await check(), "default metadata");

      await program.methods
        .setSpecificFeedMetadata(
          [Array.from(id)],
          [{ ...workflow, allowedWorkflowName: Array.from(randomBytes(10)) }]
        )
        .accountsStrict(adminAccounts())
        .remainingAccounts([meta(feedPda(id))])
        .signers([feedAdmin.keypair])
        .rpc();
      assert.isFalse(await check(), "specific metadata replaces the default");
    });
  });

  describe("reports", function () {
    beforeEach(async () => initStore(4));

    it("stores reports and serves rounds at 18 decimals", async () => {
      const a = newDataId();
      const b = newDataId();
      await setupFeed(a, b);
      await expectError(readLatest(a), "NoDataPresent");

      const tsA = nextTimestamp();
      const tsB = nextTimestamp();
      await report([
        { dataId: a, timestamp: tsA, answer: new BN(1234).mul(E18).divn(100) },
        { dataId: b, timestamp: tsB, answer: new BN(-5).mul(E18) },
      ]);

      const round = await readLatest(a);
      assert.equal(round.roundId.toNumber(), 1);
      assert.isTrue(round.answer.eq(new BN(1234).mul(E18).divn(100)));
      assert.equal(round.observationsTimestamp, tsA);
      assert.isAbove(round.recordedTimestamp, 0);
      assert.isTrue(
        (await latestAnswer(b)).eq(new BN(-5).mul(E18)),
        "negative"
      );
      assert.equal((await readLatest(b)).observationsTimestamp, tsB);
    });

    it("skips invalid data ids and uninitialized feeds without failing the batch", async () => {
      const valid = newDataId();
      const uninitialized = newDataId();
      const svr = withSvrPath(newDataId(), 1);
      await setupFeed(valid);

      const ts = nextTimestamp();
      const signature = await report([
        { dataId: svr, timestamp: ts, answer: new BN(1) },
        { dataId: uninitialized, timestamp: ts, answer: new BN(2) },
        { dataId: valid, timestamp: ts, answer: new BN(3) },
      ]);
      assert.isTrue(
        (await latestAnswer(valid)).eq(new BN(3)),
        "valid report lands"
      );

      const names = (await cpiEvents(signature)).map((e) => e.name);
      assert.includeMembers(names, [
        "invalidDataIdIgnored",
        "feedNotInitializedIgnored",
        "feedUpdated",
      ]);
    });

    it("reads the latest rounds of several data ids", async () => {
      const a = newDataId();
      const b = newDataId();
      await setupFeed(a, b);
      const ts = nextTimestamp();
      await report([{ dataId: a, timestamp: ts, answer: new BN(42) }]);

      const rounds = (await program.methods
        .getLatestRounds([Array.from(a), Array.from(b)])
        .accountsStrict({ state: state.publicKey })
        .remainingAccounts([meta(feedPda(a), false), meta(feedPda(b), false)])
        .view())!;
      assert.equal(rounds.length, 2);
      assert.equal(rounds[0].roundId.toNumber(), 1);
      assert.isTrue(rounds[0].answer.eq(new BN(42)));
      assert.equal(rounds[0].observationsTimestamp, ts);
      assert.equal(
        rounds[1].roundId.toNumber(),
        0,
        "no report yet: zero round"
      );
    });

    it("keeps history in a ring", async () => {
      const id = newDataId();
      await setupFeed(id);
      for (let i = 1; i <= 5; i++) {
        await report([
          { dataId: id, timestamp: nextTimestamp(), answer: new BN(i) },
        ]);
      }

      assert.isTrue((await readRound(id, 5)).answer.eq(new BN(5)));
      assert.isTrue((await readRound(id, 2)).answer.eq(new BN(2)));
      assert.equal((await readLatest(id)).roundId.toNumber(), 5);
      await expectError(readRound(id, 1), "NoDataPresent"); // evicted from a ring of 4
      await expectError(readRound(id, 6), "NoDataPresent"); // not yet written
      await expectError(readRound(id, 0), "NoDataPresent");
    });

    it("skips stale, out of range and unpermitted reports", async () => {
      const id = newDataId();
      await setupFeed(id);
      const ts = nextTimestamp();
      await report([{ dataId: id, timestamp: ts, answer: new BN(10) }]);

      await report([{ dataId: id, timestamp: ts, answer: new BN(11) }]);
      assert.isTrue((await latestAnswer(id)).eq(new BN(10)), "stale");

      await program.methods
        .setMinMaxAnswers([Array.from(id)], [new BN(0)], [new BN(100)])
        .accountsStrict(adminAccounts())
        .remainingAccounts([meta(feedPda(id))])
        .signers([feedAdmin.keypair])
        .rpc();
      await report([
        { dataId: id, timestamp: nextTimestamp(), answer: new BN(101) },
      ]);
      assert.isTrue((await latestAnswer(id)).eq(new BN(10)), "out of range");

      await program.methods
        .setSpecificFeedMetadata(
          [Array.from(id)],
          [
            {
              allowedSender: forwarderAuthority,
              allowedWorkflowOwner: Array.from(workflowOwner),
              allowedWorkflowName: Array.from(randomBytes(10)),
            },
          ]
        )
        .accountsStrict(adminAccounts())
        .remainingAccounts([meta(feedPda(id))])
        .signers([feedAdmin.keypair])
        .rpc();
      await report([
        { dataId: id, timestamp: nextTimestamp(), answer: new BN(50) },
      ]);
      assert.isTrue((await latestAnswer(id)).eq(new BN(10)), "not permitted");

      await program.methods
        .removeSpecificFeedMetadata([Array.from(id)])
        .accountsStrict(adminAccounts())
        .remainingAccounts([meta(feedPda(id))])
        .signers([feedAdmin.keypair])
        .rpc();
      await report([
        { dataId: id, timestamp: nextTimestamp(), answer: new BN(50) },
      ]);
      assert.isTrue(
        (await latestAnswer(id)).eq(new BN(50)),
        "default metadata again"
      );
    });

    it("accepts any forwarder in the allowed workflows, e.g. one per zone", async () => {
      const id = newDataId();
      await setupFeed(id);
      const zoneB = new Forwarder(forwarderProgram, provider)
        .withState(Keypair.generate())
        .withOracles(1, 21, 1);
      await zoneB.initialize();
      await zoneB.initOraclesConfig();
      const zoneBAuthority = calculateForwarderAuthorityBump(
        zoneB.state.publicKey,
        program.programId,
        forwarderProgram.programId
      )[0];

      await report(
        [{ dataId: id, timestamp: nextTimestamp(), answer: new BN(7) }],
        new Map(),
        zoneB
      );
      await expectError(readLatest(id), "NoDataPresent"); // a forwarder outside the allowed workflows is skipped

      const workflow = (allowedSender: PublicKey) => ({
        allowedSender,
        allowedWorkflowOwner: Array.from(workflowOwner),
        allowedWorkflowName: Array.from(workflowName),
      });
      await program.methods
        .setDefaultFeedMetadata([
          workflow(forwarderAuthority),
          workflow(zoneBAuthority),
        ])
        .accountsStrict({
          feedAdmin: feedAdmin.keypair.publicKey,
          state: state.publicKey,
          ...eventAccounts(),
        })
        .signers([feedAdmin.keypair])
        .rpc();
      await report(
        [{ dataId: id, timestamp: nextTimestamp(), answer: new BN(8) }],
        new Map(),
        zoneB
      );
      assert.isTrue((await latestAnswer(id)).eq(new BN(8)), "zone B");
      await report([
        { dataId: id, timestamp: nextTimestamp(), answer: new BN(9) },
      ]);
      assert.isTrue((await latestAnswer(id)).eq(new BN(9)), "zone A");
    });

    it("grows the ring before it wraps", async () => {
      const id = newDataId();
      await setupFeed(id);
      await report([
        { dataId: id, timestamp: nextTimestamp(), answer: new BN(1) },
      ]);

      await program.methods
        .setMaxStoredReports([Array.from(id)], [new BN(64)])
        .accountsStrict(adminAccounts())
        .remainingAccounts([meta(feedPda(id))])
        .signers([feedAdmin.keypair])
        .rpc();
      const feed = await program.account.feedData.fetch(feedPda(id));
      assert.equal(feed.maxStoredReports.toNumber(), 64);

      await expectError(
        program.methods
          .setMaxStoredReports([Array.from(id)], [new BN(32)])
          .accountsStrict(adminAccounts())
          .remainingAccounts([meta(feedPda(id))])
          .signers([feedAdmin.keypair])
          .rpc(),
        "MaxStoredReportsNotIncreased"
      );

      assert.isTrue(
        (await readRound(id, 1)).answer.eq(new BN(1)),
        "history survives the resize"
      );
    });
  });

  describe("report callbacks", function () {
    beforeEach(async () => initStore(16));

    const allowCallbackProgram = (programId: PublicKey, allowed = true) =>
      program.methods
        .setCallbackProgramAllowed(programId, allowed)
        .accountsStrict({
          owner: provider.publicKey,
          state: state.publicKey,
          allowlistEntry: callbackProgramEntry(programId),
          systemProgram: anchor.web3.SystemProgram.programId,
          ...eventAccounts(),
        })
        .rpc();
    const configureCallback = (
      dataId: Buffer,
      programId: PublicKey,
      accounts: { pubkey: PublicKey; isWritable: boolean }[],
      revertOnFailure: boolean
    ) =>
      program.methods
        .setReportCallbackConfigs(
          [Array.from(dataId)],
          [{ program: programId, accounts, revertOnFailure }]
        )
        .accountsStrict(adminAccounts())
        .remainingAccounts([
          meta(feedPda(dataId)),
          meta(programId, false),
          meta(callbackProgramEntry(programId), false),
        ])
        .signers([feedAdmin.keypair])
        .rpc();
    const setCallback = async (
      dataId: Buffer,
      programId: PublicKey,
      accounts: { pubkey: PublicKey; isWritable: boolean }[],
      revertOnFailure: boolean
    ) => {
      await allowCallbackProgram(programId);
      return configureCallback(dataId, programId, accounts, revertOnFailure);
    };
    // the DF1 store's callback emits CPI events: its event authority and program follow the feed
    const legacyCallbackAccounts = (legacy: PublicKey) => [
      { pubkey: legacy, isWritable: true },
      { pubkey: eventAuthority(storeProgram.programId), isWritable: false },
      { pubkey: storeProgram.programId, isWritable: false },
    ];
    const reportAccountsFor = (
      programId: PublicKey,
      accounts: { pubkey: PublicKey; isWritable: boolean }[]
    ) => [
      meta(programId, false),
      meta(callbackAuthority(programId), false),
      ...accounts.map((a) => meta(a.pubkey, a.isWritable)),
    ];

    // DF1 feed with a single-slot live buffer
    const createLegacyFeed = async (decimals: number) => {
      const feed = Keypair.generate();
      await storeProgram.methods
        .createFeed("legacy feed", decimals, 1, 1)
        .accounts({ feed: feed.publicKey, authority: provider.publicKey })
        .signers([feed])
        .preInstructions([
          await storeProgram.account.transmissions.createInstruction(
            feed,
            8 + 192 + 48
          ),
        ])
        .rpc();
      return feed.publicKey;
    };
    const setWriter = (feed: PublicKey, writer: PublicKey) =>
      storeProgram.methods
        .setWriter(writer)
        .accounts({
          feed,
          owner: provider.publicKey,
          authority: provider.publicKey,
        })
        .rpc();
    const legacyRound = async (feed: PublicKey) => {
      const header = await storeProgram.account.transmissions.fetch(feed);
      const live = (await connection.getAccountInfo(feed)).data.subarray(
        200,
        248
      );
      return {
        roundId: header.latestRoundId,
        timestamp: live.readUInt32LE(8),
        answer: new BN(live.subarray(16, 32), "le").fromTwos(128),
      };
    };

    it("writes through to a repointed DF1 feed", async () => {
      const id = newDataId();
      await setupFeed(id);
      const legacy = await createLegacyFeed(8);
      await setCallback(
        id,
        storeProgram.programId,
        legacyCallbackAccounts(legacy),
        false
      );
      const callbackAccounts = new Map([
        [
          id.toString("hex"),
          reportAccountsFor(
            storeProgram.programId,
            legacyCallbackAccounts(legacy)
          ),
        ],
      ]);

      // writer still OCR2: the store is written, the legacy feed is skipped
      await report(
        [
          {
            dataId: id,
            timestamp: nextTimestamp(),
            answer: new BN(2).mul(E18),
          },
        ],
        callbackAccounts
      );
      assert.isTrue((await latestAnswer(id)).eq(new BN(2).mul(E18)));
      assert.equal((await legacyRound(legacy)).roundId, 0);

      // repoint the legacy feed to the DF1 store program's callback authority
      await setWriter(legacy, callbackAuthority(storeProgram.programId));
      const ts = nextTimestamp();
      await report(
        [{ dataId: id, timestamp: ts, answer: new BN(105).mul(E18).divn(100) }],
        callbackAccounts
      );
      const round = await legacyRound(legacy);
      assert.equal(round.roundId, 1);
      assert.equal(round.timestamp, ts);
      assert.isTrue(
        round.answer.eq(new BN(105).mul(E8).divn(100)),
        "rescaled to the feed decimals"
      );

      // rollback: point the writer elsewhere
      await setWriter(legacy, Keypair.generate().publicKey);
      await report(
        [
          {
            dataId: id,
            timestamp: nextTimestamp(),
            answer: new BN(4).mul(E18),
          },
        ],
        callbackAccounts
      );
      assert.equal((await legacyRound(legacy)).roundId, 1);
      assert.isTrue((await latestAnswer(id)).eq(new BN(4).mul(E18)));
    });

    it("drops reports a callback rejects when configured to", async () => {
      const id = newDataId();
      await setupFeed(id);
      const callbackAccounts = new Map([
        [id.toString("hex"), reportAccountsFor(dummyReceiver.programId, [])],
      ]);

      // the mock callback rejects negative answers
      await setCallback(id, dummyReceiver.programId, [], true);
      await report(
        [{ dataId: id, timestamp: nextTimestamp(), answer: new BN(5) }],
        callbackAccounts
      );
      await report(
        [{ dataId: id, timestamp: nextTimestamp(), answer: new BN(-5) }],
        callbackAccounts
      );
      assert.isTrue(
        (await latestAnswer(id)).eq(new BN(5)),
        "rejected report dropped"
      );

      await setCallback(id, dummyReceiver.programId, [], false);
      await report(
        [{ dataId: id, timestamp: nextTimestamp(), answer: new BN(-6) }],
        callbackAccounts
      );
      assert.isTrue(
        (await latestAnswer(id)).eq(new BN(-6)),
        "rejection ignored"
      );
    });

    it("only configures callback programs the owner allowed", async () => {
      const id = newDataId();
      await setupFeed(id);
      await expectError(
        configureCallback(id, dummyReceiver.programId, [], false),
        "ReportCallbackNotAllowed"
      );
      await allowCallbackProgram(dummyReceiver.programId);
      await allowCallbackProgram(dummyReceiver.programId, false);
      await expectError(
        configureCallback(id, dummyReceiver.programId, [], false),
        "ReportCallbackNotAllowed"
      );
      await allowCallbackProgram(dummyReceiver.programId);
      await configureCallback(id, dummyReceiver.programId, [], false);
    });

    it("gives each callback program its own authority", async () => {
      // a DF1 feed whose writer is another callback program's authority is not written by the
      // DF1 store's callback: one program's signer never acts for another
      const id = newDataId();
      await setupFeed(id);
      const legacy = await createLegacyFeed(8);
      await setWriter(legacy, callbackAuthority(dummyReceiver.programId));
      await setCallback(
        id,
        storeProgram.programId,
        legacyCallbackAccounts(legacy),
        false
      );
      await report(
        [
          {
            dataId: id,
            timestamp: nextTimestamp(),
            answer: new BN(3).mul(E18),
          },
        ],
        new Map([
          [
            id.toString("hex"),
            reportAccountsFor(
              storeProgram.programId,
              legacyCallbackAccounts(legacy)
            ),
          ],
        ])
      );
      assert.isTrue((await latestAnswer(id)).eq(new BN(3).mul(E18)));
      assert.equal((await legacyRound(legacy)).roundId, 0, "DF1 feed skipped");
    });
  });
});
