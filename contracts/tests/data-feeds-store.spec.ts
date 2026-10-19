import * as anchor from "@coral-xyz/anchor";
import { Program, BN } from "@coral-xyz/anchor";
import { AccountMeta, Keypair, PublicKey } from "@solana/web3.js";
import { assert, expect } from "chai";
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

// compact feed id (8) | index (1) | svr path id (2) | instance (1) | reserved (4)
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
const nonSvr = (id: Buffer) => withSvrPath(id, 0);
const isSvr = (id: Buffer) => id.readUInt16BE(9) !== 0;
const indexedFeedId = (id: Buffer) => {
  const out = nonSvr(id);
  out[11] = 0;
  out.fill(0, 12);
  return out;
};

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

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
  const feedPda = (id: Buffer) => pda("feed", nonSvr(id));
  const configPda = (id: Buffer) => pda("data_id", id);
  // a non-SVR data ID's settings live in its feed, an SVR path's in its config
  const settingsPda = (id: Buffer) => (isSvr(id) ? configPda(id) : feedPda(id));
  const svrConfigPda = (id: Buffer) => (isSvr(id) ? configPda(id) : null);
  const proxyPda = (proxyId: Buffer) => pda("proxy", proxyId);
  const storeAuthority = () => pda("store_authority");

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
      })
      .signers([feedAdmin.keypair])
      .rpc();
  };

  const adminAccounts = () => ({
    feedAdmin: feedAdmin.keypair.publicKey,
    state: state.publicKey,
    systemProgram: anchor.web3.SystemProgram.programId,
  });

  // feed history for the non-SVR id plus a config for every SVR path
  const setupFeed = async (...dataIds: Buffer[]) => {
    const feeds = [
      ...new Map(
        dataIds.map((id) => [nonSvr(id).toString("hex"), nonSvr(id)])
      ).values(),
    ];
    await program.methods
      .initFeeds(feeds.map((id) => Array.from(id)))
      .accountsStrict(adminAccounts())
      .remainingAccounts(feeds.map((id) => meta(feedPda(id))))
      .signers([feedAdmin.keypair])
      .rpc();
    const svrIds = dataIds.filter(isSvr);
    if (svrIds.length === 0) return;
    await program.methods
      .initDataIds(svrIds.map((id) => Array.from(id)))
      .accountsStrict(adminAccounts())
      .remainingAccounts(svrIds.map((id) => meta(configPda(id))))
      .signers([feedAdmin.keypair])
      .rpc();
  };

  const setProxy = async (
    proxyId: Buffer,
    dataId: Buffer,
    decimals: number,
    fallbackTime = 0,
    maxSyncIterations = 0
  ) => {
    await program.methods
      .updateConfigsForProxies(
        [Array.from(proxyId)],
        [
          {
            dataId: Array.from(dataId),
            decimals,
            fallbackTime,
            maxSyncIterations,
          },
        ]
      )
      .accountsStrict(adminAccounts())
      .remainingAccounts([meta(proxyPda(proxyId)), meta(feedPda(dataId))])
      .signers([feedAdmin.keypair])
      .rpc();
    return proxyPda(proxyId);
  };

  // per report: data id config for an SVR path, non-SVR feed, then callback program and accounts
  // if configured
  const report = async (
    reports: Report[],
    callbackAccounts: Map<string, AccountMeta[]> = new Map(),
    via: Forwarder = forwarder
  ) => {
    const accounts: AccountMeta[] = [
      meta(state.publicKey, false),
      meta(storeAuthority(), false),
    ];
    for (const r of reports) {
      if (isSvr(r.dataId)) accounts.push(meta(configPda(r.dataId)));
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
    const status = await connection.getSignatureStatus(result);
    lastWriteSlot = status.value.slot;
    return result;
  };

  // wait for a later slot than the last report, so same-slot SVR exclusivity has passed
  let lastWriteSlot = 0;
  const nextSlot = async () => {
    while ((await connection.getSlot("confirmed")) <= lastWriteSlot)
      await sleep(100);
  };

  const proxyRead = (proxy: PublicKey, dataId: Buffer) => ({
    proxy,
    feed: feedPda(dataId),
    dataIdConfig: svrConfigPda(dataId),
    feedDescription: null,
    svrPathDescription: null,
  });

  const directRead = (dataId: Buffer) => ({
    state: state.publicKey,
    feed: feedPda(dataId),
    dataIdConfig: svrConfigPda(dataId),
  });

  const latestAnswer = (dataId: Buffer, fallbackTime = 0, maxSync = 0) =>
    program.methods
      .getLatestAnswer(Array.from(dataId), 18, fallbackTime, maxSync)
      .accountsStrict(directRead(dataId))
      .view() as Promise<BN>;

  const expectError = async (p: Promise<any>, name: string) => {
    try {
      await p;
    } catch (e) {
      expect(String(e)).to.include(name);
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

    it("validates proxy configs", async () => {
      const id = newDataId();
      await setupFeed(id);
      await expectError(
        setProxy(randomBytes(32), id, 19),
        "DecimalsAboveStoredDecimals"
      );
      await expectError(
        setProxy(randomBytes(32), id, 8, 9),
        "FallbackTimeAboveMaxStoredReports"
      );
      await expectError(
        setProxy(randomBytes(32), withSvrPath(id, 1), 8, 1, 0),
        "InvalidMaxSyncIterations"
      );

      const proxyId = randomBytes(32);
      const proxy = await setProxy(proxyId, id, 8);
      const config = await program.account.proxyConfig.fetch(proxy);
      assert.deepEqual(Buffer.from(config.dataId), id);
      assert.equal(config.decimals, 8);

      await program.methods
        .removeConfigsForProxies([Array.from(proxyId)])
        .accountsStrict(adminAccounts())
        .remainingAccounts([meta(proxy)])
        .signers([feedAdmin.keypair])
        .rpc();
      assert.isNull(await connection.getAccountInfo(proxy));
    });

    it("sets descriptions", async () => {
      const id = newDataId();
      const svrId = withSvrPath(id, 7);
      const feedDescription = pda("feed_description", indexedFeedId(id));
      const pathDescription = pda("svr_path_description", Buffer.from([0, 7]));

      await program.methods
        .setFeedDescriptions([Array.from(indexedFeedId(id))], ["ETH/USD"])
        .accountsStrict(adminAccounts())
        .remainingAccounts([meta(feedDescription)])
        .signers([feedAdmin.keypair])
        .rpc();
      await program.methods
        .setSvrPathDescriptions([[0, 7]], ["SVR"])
        .accountsStrict(adminAccounts())
        .remainingAccounts([meta(pathDescription)])
        .signers([feedAdmin.keypair])
        .rpc();

      const description = await program.methods
        .getDescription(Array.from(svrId))
        .accountsStrict({
          state: state.publicKey,
          feedDescription,
          svrPathDescription: pathDescription,
        })
        .view();
      assert.equal(description, "ETH/USD SVR");
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
          .accountsStrict({
            state: state.publicKey,
            settings: settingsPda(id),
          })
          .view();

      assert.isTrue(await check(), "default metadata");

      await program.methods
        .setSpecificFeedMetadata(
          [Array.from(id)],
          [{ ...workflow, allowedWorkflowName: Array.from(randomBytes(10)) }]
        )
        .accountsStrict(adminAccounts())
        .remainingAccounts([meta(settingsPda(id))])
        .signers([feedAdmin.keypair])
        .rpc();
      assert.isFalse(await check(), "specific metadata replaces the default");
    });

    it("keeps non-SVR settings in the feed, so only SVR paths get a config", async () => {
      const id = newDataId();
      await setupFeed(id);
      await expectError(
        program.methods
          .initDataIds([Array.from(id)])
          .accountsStrict(adminAccounts())
          .remainingAccounts([meta(configPda(id))])
          .signers([feedAdmin.keypair])
          .rpc(),
        "InvalidDataId"
      );
    });
  });

  describe("reports", function () {
    beforeEach(async () => initStore(4));

    it("stores non-SVR reports and serves proxies", async () => {
      const a = newDataId();
      const b = newDataId();
      await setupFeed(a, b);
      const proxy = await setProxy(randomBytes(32), a, 8);

      const tsA = nextTimestamp();
      await report([
        { dataId: a, timestamp: tsA, answer: new BN(1234).mul(E18).divn(100) },
        { dataId: b, timestamp: nextTimestamp(), answer: new BN(-5).mul(E18) },
      ]);

      const round = await program.methods
        .latestRoundData()
        .accountsStrict(proxyRead(proxy, a))
        .view();
      assert.equal(round.roundId.toNumber(), 1);
      assert.isTrue(
        round.answer.eq(new BN(1234).mul(E8).divn(100)),
        "downscaled to 8 decimals"
      );
      assert.equal(round.startedAt.toNumber(), tsA);
      assert.isTrue(
        (await latestAnswer(b)).eq(new BN(-5).mul(E18)),
        "negative answers"
      );

      assert.equal(
        await program.methods
          .decimals()
          .accountsStrict(proxyRead(proxy, a))
          .view(),
        8
      );
      assert.equal(
        (
          await program.methods
            .version()
            .accountsStrict(proxyRead(proxy, a))
            .view()
        ).toNumber(),
        1
      );
    });

    it("keeps history in a ring", async () => {
      const id = newDataId();
      await setupFeed(id);
      const proxy = await setProxy(randomBytes(32), id, 18);
      for (let i = 1; i <= 5; i++) {
        await report([
          { dataId: id, timestamp: nextTimestamp(), answer: new BN(i) },
        ]);
      }
      const roundData = (r: number) =>
        program.methods
          .getRoundData(new BN(r))
          .accountsStrict(proxyRead(proxy, id))
          .view();

      assert.isTrue((await roundData(5)).answer.eq(new BN(5)));
      assert.isTrue((await roundData(2)).answer.eq(new BN(2)));
      assert.isTrue(
        (await roundData(1)).answer.isZero(),
        "evicted from a ring of 4"
      );
      assert.isTrue((await roundData(6)).answer.isZero(), "not yet written");
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
        .remainingAccounts([meta(settingsPda(id))])
        .signers([feedAdmin.keypair])
        .rpc();
      await report([
        { dataId: id, timestamp: nextTimestamp(), answer: new BN(50) },
      ]);
      assert.isTrue((await latestAnswer(id)).eq(new BN(10)), "not permitted");

      await program.methods
        .removeSpecificFeedMetadata([Array.from(id)])
        .accountsStrict(adminAccounts())
        .remainingAccounts([meta(settingsPda(id))])
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
      assert.isTrue(
        (await latestAnswer(id)).eq(new BN(0)),
        "a forwarder outside the allowed workflows is skipped"
      );

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

      const proxy = await setProxy(randomBytes(32), id, 18);
      assert.isTrue(
        (
          await program.methods
            .getRoundData(new BN(1))
            .accountsStrict(proxyRead(proxy, id))
            .view()
        ).answer.eq(new BN(1)),
        "history survives the resize"
      );
    });
  });

  describe("SVR", function () {
    beforeEach(async () => initStore(16));

    it("appends fresher SVR reports and adopts the head otherwise", async () => {
      const id = newDataId();
      const svrId = withSvrPath(id, 1);
      await setupFeed(id, svrId);

      await report([
        { dataId: id, timestamp: nextTimestamp(), answer: new BN(100) },
      ]);
      const svrTs = nextTimestamp();
      await report([{ dataId: svrId, timestamp: svrTs, answer: new BN(111) }]);

      const config = await program.account.dataIdConfig.fetch(configPda(svrId));
      assert.equal(config.svrRoundId.toNumber(), 2, "appended round 2");
      assert.isTrue(config.svrAnswer.eq(new BN(111)));

      await nextSlot();
      assert.equal(
        (await latestAnswer(id)).toString(),
        "111",
        "released to non-SVR readers"
      );
      assert.equal((await latestAnswer(svrId)).toString(), "111");

      // a newer non-SVR report, then an SVR report behind it adopts the head
      await report([
        { dataId: id, timestamp: nextTimestamp(), answer: new BN(120) },
      ]);
      await report([
        { dataId: svrId, timestamp: svrTs + 1, answer: new BN(1) },
      ]);
      const adopted = await program.account.dataIdConfig.fetch(
        configPda(svrId)
      );
      assert.equal(adopted.svrRoundId.toNumber(), 3, "adopted the head");
      assert.isTrue(adopted.svrAnswer.eq(new BN(120)));
      const feed = await program.account.feedData.fetch(feedPda(id));
      assert.equal(feed.roundId.toNumber(), 3, "no round appended");
    });

    it("falls back to aged non-SVR rounds and honours disabled paths", async () => {
      const id = newDataId();
      const svrId = withSvrPath(id, 2);
      await setupFeed(id, svrId);

      await report([
        { dataId: svrId, timestamp: nextTimestamp(), answer: new BN(111) },
      ]);
      await report([
        { dataId: id, timestamp: nextTimestamp(), answer: new BN(120) },
      ]);
      await sleep(3_000);

      // long window: the path keeps its own selection
      assert.isTrue((await latestAnswer(svrId, 600, 5)).eq(new BN(111)));
      // short window: newest non-SVR round older than the window
      assert.isTrue((await latestAnswer(svrId, 1, 5)).eq(new BN(120)));

      await program.methods
        .setSvrDisabled([Array.from(svrId)], [true])
        .accountsStrict(adminAccounts())
        .remainingAccounts([meta(configPda(svrId))])
        .signers([feedAdmin.keypair])
        .rpc();
      assert.isTrue(
        (await latestAnswer(svrId, 600, 5)).eq(new BN(120)),
        "disabled path"
      );
    });
  });

  describe("report callbacks", function () {
    beforeEach(async () => initStore(16));

    const setCallback = async (
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
        .remainingAccounts([meta(settingsPda(dataId)), meta(programId, false)])
        .signers([feedAdmin.keypair])
        .rpc();

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
      const svrId = withSvrPath(id, 1);
      await setupFeed(id, svrId);
      const legacy = await createLegacyFeed(8);
      await setCallback(
        id,
        storeProgram.programId,
        [{ pubkey: legacy, isWritable: true }],
        false
      );
      const callbackAccounts = new Map([
        [
          id.toString("hex"),
          [meta(storeProgram.programId, false), meta(legacy)],
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

      // repoint the legacy feed to the store authority
      await setWriter(legacy, storeAuthority());
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

      // SVR rounds never reach the legacy feed
      await report([
        {
          dataId: svrId,
          timestamp: nextTimestamp(),
          answer: new BN(3).mul(E18),
        },
      ]);
      assert.equal((await legacyRound(legacy)).roundId, 1);

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
        [id.toString("hex"), [meta(dummyReceiver.programId, false)]],
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
  });
});
