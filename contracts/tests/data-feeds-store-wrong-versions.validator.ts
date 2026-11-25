import * as anchor from "@coral-xyz/anchor";
import { BN, Program } from "@coral-xyz/anchor";
import { AccountMeta, PublicKey, SystemProgram } from "@solana/web3.js";
import { DataFeedsStore } from "../target/types/data_feeds_store";
import { newSigners, Signer } from "./utils";

const fixtures = require("./data-feeds-store-wrong-versions.cjs");

describe("data feeds store account versions", function () {
  this.timeout(90_000);
  const provider = anchor.AnchorProvider.env();
  anchor.setProvider(provider);
  const connection = provider.connection;
  const program = anchor.workspace.DataFeedsStore as Program<DataFeedsStore>;
  const loader = new PublicKey("BPFLoaderUpgradeab1e11111111111111111111111");
  const programData = PublicKey.findProgramAddressSync(
    [program.programId.toBuffer()],
    loader
  )[0];
  const state = fixtures.statePda(fixtures.STORE_ID) as PublicKey;
  let feedAdmin: Signer;

  const eventAccounts = () => ({
    eventAuthority: PublicKey.findProgramAddressSync(
      [Buffer.from("__event_authority")],
      program.programId
    )[0],
    program: program.programId,
  });
  const meta = (pubkey: PublicKey): AccountMeta => ({
    pubkey,
    isSigner: false,
    isWritable: true,
  });
  const adminAccounts = (storeState: PublicKey = state) => ({
    feedAdmin: feedAdmin.keypair.publicKey,
    state: storeState,
    systemProgram: SystemProgram.programId,
    ...eventAccounts(),
  });
  const asFeedAdmin = (
    call: {
      accountsStrict(accounts: ReturnType<typeof adminAccounts>): {
        remainingAccounts(accounts: AccountMeta[]): {
          signers(signers: Signer["keypair"][]): { rpc(): Promise<string> };
        };
      };
    },
    remaining: AccountMeta[],
    storeState?: PublicKey
  ) =>
    call
      .accountsStrict(adminAccounts(storeState))
      .remainingAccounts(remaining)
      .signers([feedAdmin.keypair])
      .rpc();
  const read = (storeState: PublicKey, id: Buffer) => ({
    state: storeState,
    feed: fixtures.feedPda(storeState, id) as PublicKey,
  });
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
      if (text.includes(name)) return;
      throw e;
    }
    throw new Error(`expected ${name}`);
  };

  before(async () => {
    [feedAdmin] = await newSigners(connection, 1);
    await program.methods
      .initialize(new BN(fixtures.STORE_ID), {
        feedAdmins: [feedAdmin.keypair.publicKey],
        defaultMaxStoredReports: new BN(4),
        maxObservationForwardSecs: 3600,
        df1Program: fixtures.DF1_FEED,
      })
      .accountsStrict({
        owner: provider.publicKey,
        state,
        programData,
        systemProgram: SystemProgram.programId,
        ...eventAccounts(),
      })
      .rpc();
  });

  it("rejects a wrong store-state version in read and admin instructions", async () => {
    const wrongState = fixtures.statePda(
      fixtures.WRONG_STATE_STORE_ID
    ) as PublicKey;
    await expectError(
      program.methods
        .latestRound(Array.from(fixtures.DATA_ID), 18)
        .accountsStrict(read(wrongState, fixtures.DATA_ID))
        .view(),
      "InvalidAccountVersion"
    );
    await expectError(
      asFeedAdmin(
        program.methods.setMaxObservationForwardDuration(1),
        [],
        wrongState
      ),
      "InvalidAccountVersion"
    );
  });

  it("rejects a wrong feed version in read and admin instructions", async () => {
    await expectError(
      program.methods
        .latestRound(Array.from(fixtures.DATA_ID), 18)
        .accountsStrict(read(state, fixtures.DATA_ID))
        .view(),
      "InvalidAccountVersion"
    );
    await expectError(
      asFeedAdmin(
        program.methods.setFeedDescriptions(
          [Array.from(fixtures.DATA_ID)],
          ["wrong version"]
        ),
        [meta(fixtures.feedPda(state, fixtures.DATA_ID))]
      ),
      "InvalidAccountVersion"
    );
  });

  it("rejects a wrong DF1 link-marker version in admin instructions", async () => {
    await expectError(
      asFeedAdmin(
        program.methods.setDf1Feeds(
          [Array.from(fixtures.LINK_DATA_ID)],
          [PublicKey.default],
          [false]
        ),
        [
          meta(fixtures.feedPda(state, fixtures.LINK_DATA_ID)),
          meta(fixtures.linkPda(state, fixtures.DF1_FEED)),
        ]
      ),
      "InvalidAccountVersion"
    );
  });
});
