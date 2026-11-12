import * as anchor from "@coral-xyz/anchor";
import { Program, BN } from "@coral-xyz/anchor";
import { AccountMeta, Keypair, PublicKey, SystemProgram } from "@solana/web3.js";
import { assert } from "chai";
import { randomBytes } from "crypto";
import { DataFeedsProxy } from "../target/types/data_feeds_proxy";
import { newSigners, Signer } from "./utils";

const E18 = new BN(10).pow(new BN(18));
const E8 = new BN(10).pow(new BN(8));

// compact feed id (8) | index (1) | svr path id (2, reserved) | instance (1) | reserved (4)
const newDataId = () => {
  const id = Buffer.alloc(16);
  randomBytes(8).copy(id, 0);
  return id;
};
// accounts key data ids zero padded to 32 bytes
const seed32 = (id: Buffer) => Buffer.concat([id, Buffer.alloc(16)]);

// The proxy on its own: a keypair stands in for the trusted store, so every writer-signed
// instruction can be driven directly. The store spec covers the store writing through it.
describe("data feeds proxy", function () {
  this.timeout(60_000);
  const provider = anchor.AnchorProvider.env();
  anchor.setProvider(provider);
  const connection = provider.connection;

  const proxy = anchor.workspace.DataFeedsProxy as Program<DataFeedsProxy>;

  let writer: Signer;
  let other: Signer;
  let state: Keypair;

  const feedPda = (id: Buffer, proxyState = state.publicKey) =>
    PublicKey.findProgramAddressSync(
      [Buffer.from("feed"), proxyState.toBuffer(), seed32(id)],
      proxy.programId
    );
  const eventAccounts = () => ({
    eventAuthority: PublicKey.findProgramAddressSync(
      [Buffer.from("__event_authority")],
      proxy.programId
    )[0],
    program: proxy.programId,
  });
  const meta = (pubkey: PublicKey, isWritable = true): AccountMeta => ({
    pubkey,
    isSigner: false,
    isWritable,
  });
  const programData = PublicKey.findProgramAddressSync(
    [proxy.programId.toBuffer()],
    new PublicKey("BPFLoaderUpgradeab1e11111111111111111111111")
  )[0];

  const createStateAccount = async (account: Keypair) => {
    const space = proxy.account.proxyState.size;
    return SystemProgram.createAccount({
      fromPubkey: provider.publicKey,
      newAccountPubkey: account.publicKey,
      space,
      lamports: await connection.getMinimumBalanceForRentExemption(space),
      programId: proxy.programId,
    });
  };
  const initialize = (owner: PublicKey, account: PublicKey, w: PublicKey) =>
    proxy.methods
      .initialize(w)
      .accountsStrict({ owner, state: account, programData, ...eventAccounts() });

  const initProxy = async () => {
    state = Keypair.generate();
    await initialize(provider.publicKey, state.publicKey, writer.keypair.publicKey)
      .preInstructions([await createStateAccount(state)])
      .signers([state])
      .rpc();
  };

  const writerPayer = (w = writer) => ({
    writer: w.keypair.publicKey,
    state: state.publicKey,
    payer: w.keypair.publicKey,
    systemProgram: SystemProgram.programId,
  });
  const createFeeds = (ids: Buffer[], maxStoredReports = 3, w = writer) =>
    proxy.methods
      .createFeeds(
        ids.map((id) => Array.from(id)),
        Buffer.from(ids.map((id) => feedPda(id)[1])),
        new BN(maxStoredReports)
      )
      .accountsStrict(writerPayer(w))
      .remainingAccounts(ids.map((id) => meta(feedPda(id)[0])))
      .signers([w.keypair]);
  const writeRounds = (
    rounds: { id: Buffer; ts: number; answer: BN }[],
    w = writer
  ) =>
    proxy.methods
      .writeRounds(
        rounds.map((r) => ({ observationsTimestamp: r.ts, answer: r.answer }))
      )
      .accountsStrict({ writer: w.keypair.publicKey, state: state.publicKey })
      .remainingAccounts(rounds.map((r) => meta(feedPda(r.id)[0])))
      .signers([w.keypair]);
  const read = (id: Buffer) => ({ state: state.publicKey, feed: feedPda(id)[0] });
  const latest = (id: Buffer, decimals = 18) =>
    proxy.methods.latestRound(Array.from(id), decimals).accountsStrict(read(id)).view();
  const round = (id: Buffer, roundId: number, decimals = 18) =>
    proxy.methods
      .getRound(Array.from(id), new BN(roundId), decimals)
      .accountsStrict(read(id))
      .view();

  const expectError = async (p: Promise<any>, name: string) => {
    try {
      await p;
    } catch (e: any) {
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
    [writer, other] = await newSigners(connection, 2);
  });
  beforeEach(initProxy);

  it("initializes, by the upgrade authority only", async () => {
    const s = await proxy.account.proxyState.fetch(state.publicKey);
    assert.isTrue(s.owner.equals(provider.publicKey));
    assert.isTrue(s.writer.equals(writer.keypair.publicKey));
    assert.isTrue(
      (await proxy.methods.getStore().accountsStrict({ state: state.publicKey }).view()).equals(
        writer.keypair.publicKey
      )
    );

    const second = Keypair.generate();
    await expectError(
      initialize(other.keypair.publicKey, second.publicKey, writer.keypair.publicKey)
        .preInstructions([await createStateAccount(second)])
        .signers([second, other.keypair])
        .rpc(),
      "Unauthorized"
    );
  });

  it("gives every error its own code", () => {
    const codes = proxy.idl.errors!.map((e) => e.code);
    assert.equal(new Set(codes).size, codes.length);
  });

  it("creates feeds once, at their address, for the writer only", async () => {
    const id = newDataId();
    await expectError(createFeeds([id], 3, other).rpc(), "Unauthorized");
    await createFeeds([id]).rpc();
    const feed = await proxy.account.feed.fetch(feedPda(id)[0]);
    assert.equal(feed.version, 1);
    assert.equal(feed.minDecimals, 18, "strictest minimum by default");
    assert.equal(feed.maxStoredReports.toNumber(), 3);
    assert.isTrue(feed.state.equals(state.publicKey));
    assert.deepEqual(Buffer.from(feed.dataId), seed32(id));

    // repeating it leaves the feed as it is
    await writeRounds([{ id, ts: 10, answer: new BN(1) }]).rpc();
    await createFeeds([id]).rpc();
    assert.equal((await proxy.account.feed.fetch(feedPda(id)[0])).roundId.toNumber(), 1);

    // another data id's address
    const wrong = newDataId();
    await expectError(
      proxy.methods
        .createFeeds([Array.from(wrong)], Buffer.from([feedPda(wrong)[1]]), new BN(3))
        .accountsStrict(writerPayer())
        .remainingAccounts([meta(feedPda(newDataId())[0])])
        .signers([writer.keypair])
        .rpc(),
      "AccountMismatch"
    );
    await expectError(createFeeds([newDataId()], 1).rpc(), "InvalidMaxStoredReports");
  });

  it("appends rounds the writer sends, newer ones only", async () => {
    const [a, b] = [newDataId(), newDataId()];
    await createFeeds([a, b]).rpc();
    await expectError(latest(a), "NoDataPresent");

    // a feed may appear twice in one write
    await writeRounds([
      { id: a, ts: 10, answer: new BN(1).mul(E18) },
      { id: b, ts: 10, answer: new BN(-2).mul(E18) },
      { id: a, ts: 11, answer: new BN(3).mul(E18) },
    ]).rpc();
    const r = await latest(a);
    assert.equal(r.roundId.toNumber(), 2);
    assert.isTrue(r.answer.eq(new BN(3).mul(E18)));
    assert.equal(r.observationsTimestamp, 11);
    assert.isAbove(r.recordedTimestamp, 0);
    assert.isTrue((await latest(b)).answer.eq(new BN(-2).mul(E18)));

    await expectError(writeRounds([{ id: a, ts: 11, answer: new BN(1) }]).rpc(), "StaleRound");
    await expectError(
      writeRounds([{ id: a, ts: 12, answer: new BN(1) }], other).rpc(),
      "Unauthorized"
    );
    // a feed of another proxy
    const otherState = state;
    await initProxy();
    await expectError(
      proxy.methods
        .writeRounds([{ observationsTimestamp: 12, answer: new BN(1) }])
        .accountsStrict({ writer: writer.keypair.publicKey, state: state.publicKey })
        .remainingAccounts([meta(feedPda(a, otherState.publicKey)[0])])
        .signers([writer.keypair])
        .rpc(),
      "AccountMismatch"
    );
  });

  it("keeps history in a ring and grows it until it wraps", async () => {
    const id = newDataId();
    await createFeeds([id], 2).rpc();
    await writeRounds([{ id, ts: 1, answer: new BN(1) }]).rpc();

    const grow = (max: number, w = writer) =>
      proxy.methods
        .setMaxStoredReports([Array.from(id)], [new BN(max)])
        .accountsStrict(writerPayer(w))
        .remainingAccounts([meta(feedPda(id)[0])])
        .signers([w.keypair]);
    await expectError(grow(5, other).rpc(), "Unauthorized");
    await expectError(grow(2).rpc(), "MaxStoredReportsNotIncreased");
    await grow(3).rpc();

    for (let ts = 2; ts <= 4; ts++) {
      await writeRounds([{ id, ts, answer: new BN(ts) }]).rpc();
    }
    // rounds 2..=4 kept
    assert.isTrue((await round(id, 2)).answer.eq(new BN(2)));
    assert.isTrue((await round(id, 4)).answer.eq(new BN(4)));
    await expectError(round(id, 1), "NoDataPresent");
    await expectError(round(id, 5), "NoDataPresent");
    await expectError(grow(9).rpc(), "MaxStoredReportsLocked");
  });

  it("reads at the caller's decimals, down to the feed's minimum", async () => {
    const id = newDataId();
    await createFeeds([id]).rpc();
    await writeRounds([{ id, ts: 1, answer: new BN(123456789).mul(E18).divn(100) }]).rpc();

    assert.equal(
      await proxy.methods.decimals(Array.from(id)).accountsStrict(read(id)).view(),
      18
    );
    await expectError(latest(id, 8), "InvalidDecimals");
    await expectError(latest(id, 19), "InvalidDecimals");

    const setMin = (min: number, owner = provider.publicKey) =>
      proxy.methods
        .setMinDecimals(Array.from(id), min)
        .accountsStrict({
          owner,
          state: state.publicKey,
          feed: feedPda(id)[0],
          ...eventAccounts(),
        });
    await expectError(
      setMin(8, writer.keypair.publicKey).signers([writer.keypair]).rpc(),
      "Unauthorized"
    );
    await setMin(0).rpc();
    assert.equal(
      await proxy.methods.getMinDecimals(Array.from(id)).accountsStrict(read(id)).view(),
      0
    );
    const r = await latest(id, 8);
    assert.isTrue(r.answer.eq(new BN(123456789).mul(E8).divn(100)), "rescaled");
    assert.isTrue((await round(id, 1, 0)).answer.eq(new BN(1234567)), "truncated");

    await writeRounds([{ id, ts: 2, answer: new BN(1) }]).rpc();
    await expectError(latest(id, 8), "RoundsToZero");
  });

  it("freezes reads, not writes", async () => {
    const id = newDataId();
    await createFeeds([id]).rpc();
    await writeRounds([{ id, ts: 1, answer: new BN(1) }]).rpc();
    const freeze = (frozen: boolean, w = writer) =>
      proxy.methods
        .setFeedFrozen([Array.from(id)], frozen)
        .accountsStrict({ writer: w.keypair.publicKey, state: state.publicKey })
        .remainingAccounts([meta(feedPda(id)[0])])
        .signers([w.keypair]);
    await expectError(freeze(true, other).rpc(), "Unauthorized");
    await freeze(true).rpc();
    await expectError(latest(id), "FeedFrozen");
    await expectError(round(id, 1), "FeedFrozen");

    await writeRounds([{ id, ts: 2, answer: new BN(2) }]).rpc();
    await freeze(false).rpc();
    const r = await latest(id);
    assert.equal(r.roundId.toNumber(), 2, "written while frozen");
  });

  it("keeps descriptions in the feed account", async () => {
    const id = newDataId();
    await createFeeds([id]).rpc();
    const describe = (text: string, w = writer) =>
      proxy.methods
        .setFeedDescriptions([Array.from(id)], [text])
        .accountsStrict({ writer: w.keypair.publicKey, state: state.publicKey })
        .remainingAccounts([meta(feedPda(id)[0])])
        .signers([w.keypair]);
    const description = () =>
      proxy.methods.description(Array.from(id)).accountsStrict(read(id)).view();
    assert.equal(await description(), "");
    await describe("ETH / USD").rpc();
    assert.equal(await description(), "ETH / USD");
    await expectError(describe("x".repeat(65)).rpc(), "DescriptionTooLong");
    await expectError(describe("ETH\u0000USD").rpc(), "InvalidDescription");
    await expectError(describe("BTC / USD", other).rpc(), "Unauthorized");
  });

  it("replaces the trusted store, round ids carry on", async () => {
    const id = newDataId();
    await createFeeds([id]).rpc();
    await writeRounds([{ id, ts: 1, answer: new BN(1) }]).rpc();

    const setStore = (w: PublicKey, owner = provider.publicKey) =>
      proxy.methods
        .setStore(w)
        .accountsStrict({ owner, state: state.publicKey, ...eventAccounts() });
    await expectError(
      setStore(other.keypair.publicKey, writer.keypair.publicKey)
        .signers([writer.keypair])
        .rpc(),
      "Unauthorized"
    );
    await setStore(other.keypair.publicKey).rpc();
    await expectError(
      writeRounds([{ id, ts: 2, answer: new BN(2) }]).rpc(),
      "Unauthorized"
    );
    await writeRounds([{ id, ts: 2, answer: new BN(2) }], other).rpc();
    assert.equal((await latest(id)).roundId.toNumber(), 2, "no restart");
  });

  it("transfers ownership in two steps", async () => {
    await proxy.methods
      .transferOwnership(other.keypair.publicKey)
      .accountsStrict({ owner: provider.publicKey, state: state.publicKey, ...eventAccounts() })
      .rpc();
    await proxy.methods
      .acceptOwnership()
      .accountsStrict({
        newOwner: other.keypair.publicKey,
        state: state.publicKey,
        ...eventAccounts(),
      })
      .signers([other.keypair])
      .rpc();
    assert.isTrue(
      (await proxy.account.proxyState.fetch(state.publicKey)).owner.equals(other.keypair.publicKey)
    );
  });
});
