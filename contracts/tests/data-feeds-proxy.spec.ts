import * as anchor from "@coral-xyz/anchor";
import { Program, BN } from "@coral-xyz/anchor";
import {
  AccountMeta,
  Keypair,
  PublicKey,
  SystemProgram,
  Transaction,
} from "@solana/web3.js";
import { assert } from "chai";
import { randomBytes } from "crypto";
import { DataFeedsProxy } from "../target/types/data_feeds_proxy";
import { DataFeedsStore } from "../target/types/data_feeds_store";
import { KeystoneForwarder } from "../target/types/keystone_forwarder";
import { DummyReceiver } from "../target/types/dummy_receiver";
import {
  calculateForwarderAuthorityBump,
  Forwarder,
  newSigners,
  Signer,
} from "./utils";

const E18 = new BN(10).pow(new BN(18));
const E8 = new BN(10).pow(new BN(8));

// Borsh Vec<ReceivedReport { data_id: [u8; 16], timestamp: u32, answer: i128 }>
const encodeReport = (dataId: Buffer, timestamp: number, answer: BN) => {
  const out = Buffer.alloc(4 + 36);
  out.writeUInt32LE(1, 0);
  dataId.copy(out, 4);
  out.writeUInt32LE(timestamp, 20);
  answer.toTwos(128).toArrayLike(Buffer, "le", 16).copy(out, 24);
  return out;
};

describe("data feeds proxy", function () {
  this.timeout(60_000);
  const provider = anchor.AnchorProvider.env();
  anchor.setProvider(provider);
  const connection = provider.connection;

  const proxy = anchor.workspace.DataFeedsProxy as Program<DataFeedsProxy>;
  const store = anchor.workspace.DataFeedsStore as Program<DataFeedsStore>;
  const forwarderProgram = anchor.workspace
    .KeystoneForwarder as Program<KeystoneForwarder>;
  const dummyReceiver = anchor.workspace
    .DummyReceiver as Program<DummyReceiver>;

  let feedAdmin: Signer;
  let forwarder: Forwarder;
  let storeState: Keypair;
  let proxyState: Keypair;
  const workflowOwner = randomBytes(20);
  const workflowName = randomBytes(10);

  const storePda = (seed: string, ...keys: Buffer[]) =>
    PublicKey.findProgramAddressSync(
      [Buffer.from(seed), storeState.publicKey.toBuffer(), ...keys],
      store.programId
    )[0];
  // accounts key data ids zero padded to 32 bytes
  const seed32 = (id: Buffer) => Buffer.concat([id, Buffer.alloc(16)]);
  const feedPda = (id: Buffer) => storePda("feed", seed32(id));
  const feedConfigPda = (id: Buffer) =>
    PublicKey.findProgramAddressSync(
      [Buffer.from("feed_config"), proxyState.publicKey.toBuffer(), seed32(id)],
      proxy.programId
    )[0];
  // CPI events: #[event_cpi] contexts take the event authority PDA and the program
  const eventAuthority = (programId: PublicKey) =>
    PublicKey.findProgramAddressSync(
      [Buffer.from("__event_authority")],
      programId
    )[0];
  const proxyEvents = () => ({
    eventAuthority: eventAuthority(proxy.programId),
    program: proxy.programId,
  });
  const storeEvents = () => ({
    eventAuthority: eventAuthority(store.programId),
    program: store.programId,
  });

  const meta = (pubkey: PublicKey, isWritable = true): AccountMeta => ({
    pubkey,
    isSigner: false,
    isWritable,
  });

  let baseTimestamp: number;
  let tick = 0;
  const nextTimestamp = () => baseTimestamp + ++tick;

  const newDataId = () => {
    const id = Buffer.alloc(16);
    randomBytes(8).copy(id, 0);
    return id;
  };

  // a feed in the store with the given answers as rounds 1, 2, ...
  const feedWithRounds = async (...answers: BN[]) => {
    const id = newDataId();
    await store.methods
      .initFeeds([Array.from(id)])
      .accountsStrict({
        ...storeEvents(),
        feedAdmin: feedAdmin.keypair.publicKey,
        state: storeState.publicKey,
        systemProgram: SystemProgram.programId,
      })
      .remainingAccounts([meta(feedPda(id))])
      .signers([feedAdmin.keypair])
      .rpc();
    for (const answer of answers) {
      await forwarder.report(
        store.programId,
        encodeReport(id, nextTimestamp(), answer),
        workflowName,
        workflowOwner,
        [
          meta(storeState.publicKey, false),
          meta(eventAuthority(store.programId), false),
          meta(store.programId, false),
          meta(feedPda(id)),
        ]
      );
    }
    return id;
  };

  const readAccounts = (id: Buffer) => ({
    state: proxyState.publicKey,
    storeProgram: store.programId,
    storeState: storeState.publicKey,
    feed: feedPda(id),
  });
  const read = async (id: Buffer, roundId: number | null = null) =>
    (await proxy.methods
      .read(Array.from(id), roundId === null ? null : new BN(roundId))
      .accountsStrict(readAccounts(id))
      .view())!;
  const readScaled = async (
    id: Buffer,
    decimals: number,
    roundId: number | null = null
  ) =>
    (await proxy.methods
      .readScaled(
        Array.from(id),
        decimals,
        roundId === null ? null : new BN(roundId)
      )
      .accountsStrict({ ...readAccounts(id), feedConfig: feedConfigPda(id) })
      .view())!;
  // without the settings account, the stored decimals are the minimum
  const readScaledDefault = async (id: Buffer, decimals: number) =>
    (await proxy.methods
      .readScaled(Array.from(id), decimals, null)
      .accountsStrict({ ...readAccounts(id), feedConfig: null })
      .view())!;
  const setMinDecimals = (admin: Signer, id: Buffer, min: number) =>
    proxy.methods
      .setMinDecimals(Array.from(id), min)
      .accountsStrict({
        ...proxyEvents(),
        admin: admin.keypair.publicKey,
        state: proxyState.publicKey,
        feedConfig: feedConfigPda(id),
        systemProgram: SystemProgram.programId,
      })
      .signers([admin.keypair])
      .rpc();

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

    // the store the proxy reads from
    storeState = Keypair.generate();
    await store.methods
      .initialize([feedAdmin.keypair.publicKey], new BN(8), 3600)
      .accountsStrict({
        ...storeEvents(),
        owner: provider.publicKey,
        state: storeState.publicKey,
        systemProgram: SystemProgram.programId,
      })
      .signers([storeState])
      .rpc();
    const forwarderAuthority = calculateForwarderAuthorityBump(
      forwarder.state.publicKey,
      store.programId,
      forwarderProgram.programId
    )[0];
    await store.methods
      .setDefaultFeedMetadata([
        {
          allowedSender: forwarderAuthority,
          allowedWorkflowOwner: Array.from(workflowOwner),
          allowedWorkflowName: Array.from(workflowName),
        },
      ])
      .accountsStrict({
        ...storeEvents(),
        feedAdmin: feedAdmin.keypair.publicKey,
        state: storeState.publicKey,
      })
      .signers([feedAdmin.keypair])
      .rpc();

    const slot = await connection.getSlot();
    baseTimestamp = (await connection.getBlockTime(slot))! - 600;
  });

  beforeEach(async () => {
    proxyState = Keypair.generate();
    await proxy.methods
      .initialize([])
      .accountsStrict({
        ...proxyEvents(),
        owner: provider.publicKey,
        state: proxyState.publicKey,
        storeProgram: store.programId,
        storeState: storeState.publicKey,
        systemProgram: SystemProgram.programId,
      })
      .signers([proxyState])
      .rpc();
  });

  it("gives every error its own code", () => {
    const errors = proxy.idl.errors!;
    const codes = errors.map((e) => e.code);
    assert.equal(new Set(codes).size, codes.length);
    assert.include(
      errors.map((e) => e.name),
      "unauthorized"
    );
  });

  it("initializes", async () => {
    const s = await proxy.account.proxyState.fetch(proxyState.publicKey);
    assert.isTrue(s.owner.equals(provider.publicKey));
    assert.isTrue(s.storeProgram.equals(store.programId));
    assert.isTrue(s.storeState.equals(storeState.publicKey));
    assert.equal(s.feedAdmins.len.toNumber(), 0);
  });

  it("only points at a state account owned by the store program", async () => {
    const state = Keypair.generate();
    await expectError(
      proxy.methods
        .initialize([])
        .accountsStrict({
          ...proxyEvents(),
          owner: provider.publicKey,
          state: state.publicKey,
          storeProgram: store.programId,
          storeState: provider.publicKey, // owned by the system program
          systemProgram: SystemProgram.programId,
        })
        .signers([state])
        .rpc(),
      "ConstraintOwner"
    );
  });

  it("reads the latest or a given round at the store's decimals", async () => {
    const id = await feedWithRounds(
      new BN(1234).mul(E18).divn(100),
      new BN(-5).mul(E18)
    );

    const latest = await read(id);
    assert.equal(latest.roundId.toNumber(), 2);
    assert.isTrue(latest.answer.eq(new BN(-5).mul(E18)));
    assert.isAbove(latest.observationsTimestamp, 0);

    const first = await read(id, 1);
    assert.equal(first.roundId.toNumber(), 1);
    assert.isTrue(first.answer.eq(new BN(1234).mul(E18).divn(100)));

    await expectError(read(id, 3), "NoDataPresent");
  });

  it("fails a read of a feed without reports", async () => {
    const id = await feedWithRounds();
    await expectError(read(id), "NoDataPresent");
  });

  it("only serves the store configured in the proxy", async () => {
    const id = await feedWithRounds(new BN(7));
    await expectError(
      proxy.methods
        .read(Array.from(id), null)
        .accountsStrict({
          ...readAccounts(id),
          storeState: Keypair.generate().publicKey,
        })
        .view(),
      "ConstraintAddress"
    );
  });

  describe("scaled reads", function () {
    it("serve only the stored decimals until a minimum is set", async () => {
      const id = await feedWithRounds(new BN(105).mul(E18).divn(100));
      assert.equal(
        await proxy.methods
          .getMinDecimals(Array.from(id))
          .accountsStrict({
            state: proxyState.publicKey,
            feedConfig: null,
          })
          .view(),
        18
      );

      assert.isTrue(
        (await readScaledDefault(id, 18)).answer.eq(
          new BN(105).mul(E18).divn(100)
        )
      );
      await expectError(readScaledDefault(id, 8), "InvalidDecimals");
    });

    it("rescale to the caller's decimals within the minimum", async () => {
      const id = await feedWithRounds(
        new BN(105).mul(E18).divn(100),
        new BN(-15).mul(E18).divn(10)
      );
      await expectError(setMinDecimals(feedAdmin, id, 8), "Unauthorized");
      await setMinDecimals(
        { keypair: (provider.wallet as anchor.Wallet).payer } as Signer,
        id,
        8
      );
      assert.equal(
        await proxy.methods
          .getMinDecimals(Array.from(id))
          .accountsStrict({
            state: proxyState.publicKey,
            feedConfig: feedConfigPda(id),
          })
          .view(),
        8
      );

      const latest = await readScaled(id, 8);
      assert.isTrue(
        latest.answer.eq(new BN(-15).mul(E8).divn(10)),
        "downscaled to 8 decimals"
      );
      assert.equal(latest.roundId.toNumber(), 2);
      const first = await readScaled(id, 8, 1);
      assert.isTrue(first.answer.eq(new BN(105).mul(E8).divn(100)));
      assert.isTrue(
        (await readScaled(id, 18)).answer.eq(new BN(-15).mul(E18).divn(10)),
        "the stored decimals stay allowed"
      );

      await expectError(readScaled(id, 7), "InvalidDecimals"); // below the minimum
      await expectError(readScaled(id, 19), "InvalidDecimals"); // above the stored decimals
    });

    it("refuse an answer that would round to zero", async () => {
      const id = await feedWithRounds(new BN(10).pow(new BN(9))); // 1e-9
      await setMinDecimals(
        { keypair: (provider.wallet as anchor.Wallet).payer } as Signer,
        id,
        8
      );
      await expectError(readScaled(id, 8), "RoundsToZero");
      assert.isTrue(
        (await readScaled(id, 18)).answer.eq(new BN(10).pow(new BN(9))),
        "readable at the stored decimals"
      );
    });

    it("let feed admins, not others, set minimum decimals", async () => {
      const id = await feedWithRounds(new BN(1).mul(E18));
      await expectError(setMinDecimals(feedAdmin, id, 8), "Unauthorized");

      await proxy.methods
        .setFeedAdmin(feedAdmin.keypair.publicKey, true)
        .accountsStrict({
          ...proxyEvents(),
          owner: provider.publicKey,
          state: proxyState.publicKey,
        })
        .rpc();
      await setMinDecimals(feedAdmin, id, 6);
      assert.equal(
        await proxy.methods
          .getMinDecimals(Array.from(id))
          .accountsStrict({
            state: proxyState.publicKey,
            feedConfig: feedConfigPda(id),
          })
          .view(),
        6
      );
      await expectError(setMinDecimals(feedAdmin, id, 19), "InvalidDecimals");
    });
  });

  it("keeps its address when the store is swapped", async () => {
    const id = await feedWithRounds(new BN(7));
    assert.isTrue((await read(id)).answer.eq(new BN(7)));

    // a mock store implementing the reader interface, with a state account it owns
    const mockState = Keypair.generate();
    await provider.sendAndConfirm(
      new Transaction().add(
        SystemProgram.createAccount({
          fromPubkey: provider.publicKey,
          newAccountPubkey: mockState.publicKey,
          lamports: await connection.getMinimumBalanceForRentExemption(8),
          space: 8,
          programId: dummyReceiver.programId,
        })
      ),
      [mockState]
    );
    await expectError(
      proxy.methods
        .setStore()
        .accountsStrict({
          ...proxyEvents(),
          owner: provider.publicKey,
          state: proxyState.publicKey,
          storeProgram: dummyReceiver.programId,
          storeState: storeState.publicKey, // owned by the old store
        })
        .rpc(),
      "ConstraintOwner"
    );
    await proxy.methods
      .setStore()
      .accountsStrict({
        ...proxyEvents(),
        owner: provider.publicKey,
        state: proxyState.publicKey,
        storeProgram: dummyReceiver.programId,
        storeState: mockState.publicKey,
      })
      .rpc();

    // same proxy state, now served by the mock store
    const swapped = (await proxy.methods
      .read(Array.from(id), null)
      .accountsStrict({
        state: proxyState.publicKey,
        storeProgram: dummyReceiver.programId,
        storeState: mockState.publicKey,
        feed: feedPda(id),
      })
      .view())!;
    assert.isTrue(swapped.answer.eq(new BN(42).mul(E18)));
    assert.equal(
      (
        await proxy.account.proxyState.fetch(proxyState.publicKey)
      ).storeProgram.toBase58(),
      dummyReceiver.programId.toBase58()
    );

    // the old store's accounts are no longer accepted
    await expectError(read(id), "ConstraintAddress");
  });

  it("transfers ownership in two steps", async () => {
    const next = Keypair.generate();
    await provider.sendAndConfirm(
      new Transaction().add(
        SystemProgram.transfer({
          fromPubkey: provider.publicKey,
          toPubkey: next.publicKey,
          lamports: 10_000_000,
        })
      )
    );
    await proxy.methods
      .transferOwnership(next.publicKey)
      .accountsStrict({
        ...proxyEvents(),
        owner: provider.publicKey,
        state: proxyState.publicKey,
      })
      .rpc();
    await expectError(
      proxy.methods
        .acceptOwnership()
        .accountsStrict({
          ...proxyEvents(),
          newOwner: provider.publicKey,
          state: proxyState.publicKey,
        })
        .rpc(),
      "Unauthorized"
    );
    await proxy.methods
      .acceptOwnership()
      .accountsStrict({
        ...proxyEvents(),
        newOwner: next.publicKey,
        state: proxyState.publicKey,
      })
      .signers([next])
      .rpc();
    const s = await proxy.account.proxyState.fetch(proxyState.publicKey);
    assert.isTrue(s.owner.equals(next.publicKey));
  });
});
