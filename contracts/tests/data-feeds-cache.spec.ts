import * as anchor from "@coral-xyz/anchor";
import { struct, u32, u128, vec, u8, publicKey, array } from "@coral-xyz/borsh";
import { Program, getProvider, BN, BorshCoder } from "@coral-xyz/anchor";
import { DataFeedsCache } from "../target/types/data_feeds_cache";
import { KeystoneForwarder } from "../target/types/keystone_forwarder";
import { DummyReceiver } from "../target/types/dummy_receiver";
import {
  AccountMeta,
  Keypair,
  LAMPORTS_PER_SOL,
  PublicKey,
} from "@solana/web3.js";
// import chaiAsPromised from "chai-as-promised";
import { assert, expect } from "chai";
import {
  ArrayVec,
  arrayVecEquals,
  calculateForwarderAuthorityBump,
  Feed,
  Forwarder,
  getReportHash,
  LegacyFeedEntry,
  newFeeds,
  newSigners,
  newWorkflows,
  randomDescription,
  randomWorkflowMetadata,
  sendLamports,
  Signer,
  waitForEvent,
  WorkflowMetadata,
} from "./utils";

// chai.use(chaiAsPromised);

const workflowMetadataEq = (a: WorkflowMetadata, b: WorkflowMetadata) => {
  return (
    a.allowedSender.equals(b.allowedSender) &&
    JSON.stringify(a.allowedWorkflowName) ===
      JSON.stringify(b.allowedWorkflowName) &&
    JSON.stringify(a.allowedWorkflowOwner) ===
      JSON.stringify(b.allowedWorkflowOwner)
  );
};

describe("data feeds cache", function () {
  this.timeout(15_000);
  // Configure the client to use the local cluster.
  const provider = anchor.AnchorProvider.env();
  anchor.setProvider(provider);

  const defaultConnection = getProvider().connection;

  let feedAdminA: Signer;
  let reportSender: Signer;
  let otherSigners: Array<Signer>;

  let feedA: Feed;
  let feedB: Feed;
  let feedC: Feed;
  let feedD: Feed;
  let otherFeeds: Array<Feed>;

  const cacheProgram = anchor.workspace
    .DataFeedsCache as Program<DataFeedsCache>;

  const forwarderProgram = anchor.workspace
    .KeystoneForwarder as Program<KeystoneForwarder>;

  const mockLegacyStoreProgram = anchor.workspace
    .DummyReceiver as Program<DummyReceiver>;

  const legacyFeedsConfigPDA = (cacheState: PublicKey) => {
    const [pda, _bump] = PublicKey.findProgramAddressSync(
      [
        Buffer.from(anchor.utils.bytes.utf8.encode("legacy_feeds_config")),
        cacheState.toBuffer(),
      ],
      cacheProgram.programId
    );
    return pda;
  };

  const decimalReportPDA = (cacheState: PublicKey, dataId: Buffer) => {
    const [pda, _bump] = PublicKey.findProgramAddressSync(
      [
        Buffer.from(anchor.utils.bytes.utf8.encode("decimal_report")),
        cacheState.toBuffer(),
        dataId,
      ],
      cacheProgram.programId
    );
    return pda;
  };

  const feedConfigPDA = (cacheState: PublicKey, dataId: Buffer) => {
    const [pda, _bump] = PublicKey.findProgramAddressSync(
      [
        Buffer.from(anchor.utils.bytes.utf8.encode("feed_config")),
        cacheState.toBuffer(),
        dataId,
      ],
      cacheProgram.programId
    );
    return pda;
  };

  const permissionFlagPDA = (cacheState: PublicKey, reportHash: Buffer) => {
    const [pda, _bump] = PublicKey.findProgramAddressSync(
      [
        Buffer.from(anchor.utils.bytes.utf8.encode("permission_flag")),
        cacheState.toBuffer(),
        reportHash,
      ],
      cacheProgram.programId
    );
    return pda;
  };

  const legacyWriterPDA = (cacheState: PublicKey) => {
    const [pda, _bump] = PublicKey.findProgramAddressSync(
      [
        Buffer.from(anchor.utils.bytes.utf8.encode("legacy_writer")),
        cacheState.toBuffer(),
      ],
      cacheProgram.programId
    );
    return pda;
  };

  before(async () => {
    [feedAdminA, reportSender, ...otherSigners] = await newSigners(
      defaultConnection,
      5
    );
    [feedA, feedB, feedC, feedD, ...otherFeeds] = newFeeds(10);
  });

  describe("Initialize Cache", function () {
    it("initialize()", async () => {
      const cacheStateAccount = Keypair.generate();

      await cacheProgram.methods
        .initialize([feedAdminA.provider.publicKey])
        .accounts({
          state: cacheStateAccount.publicKey,
          owner: provider.publicKey,
          forwarderProgram: forwarderProgram.programId,
          systemProgram: anchor.web3.SystemProgram.programId,
        })
        .signers([cacheStateAccount])
        .rpc();

      const actualCacheState = await cacheProgram.account.cacheState.fetch(
        cacheStateAccount.publicKey
      );

      assert.isTrue(
        actualCacheState.owner.equals(provider.wallet.publicKey),
        "owner set"
      );

      assert.isTrue(
        actualCacheState.proposedOwner.equals(PublicKey.default),
        "proposed owner is 0"
      );

      const expectedArrayVec: ArrayVec<PublicKey> = {
        len: new BN(1),
        xs: [feedAdminA.provider.publicKey],
      };

      assert.isTrue(
        arrayVecEquals(expectedArrayVec, actualCacheState.feedAdmins, (a, b) =>
          a.equals(b)
        ),
        "feed admins equal"
      );

      const [_pda, bump] = PublicKey.findProgramAddressSync(
        [
          Buffer.from(anchor.utils.bytes.utf8.encode("legacy_writer")),
          cacheStateAccount.publicKey.toBuffer(),
        ],
        cacheProgram.programId
      );

      assert.equal(
        actualCacheState.legacyWriterBump,
        bump,
        "legacy writer bump equal"
      );
    });
  });

  describe("Legacy Feed Config Operations", function () {
    let cacheStateAccount: Keypair;
    let legacyFeedConfigAccount2: PublicKey;

    beforeEach(async () => {
      cacheStateAccount = Keypair.generate();
      legacyFeedConfigAccount2 = legacyFeedsConfigPDA(
        cacheStateAccount.publicKey
      );

      await cacheProgram.methods
        .initialize([feedAdminA.provider.publicKey])
        .accounts({
          state: cacheStateAccount.publicKey,
          owner: provider.publicKey,
          forwarderProgram: forwarderProgram.programId,
          systemProgram: anchor.web3.SystemProgram.programId,
        })
        .signers([cacheStateAccount])
        .rpc();
    });

    it("Initialize", async () => {
      const dummy = Keypair.generate();

      await cacheProgram.methods
        .initLegacyFeedsConfig([feedD.dataId] as any)
        .accounts({
          owner: provider.publicKey,
          state: cacheStateAccount.publicKey,
          legacyStore: mockLegacyStoreProgram.programId,
          legacyFeedsConfig: legacyFeedConfigAccount2,
          systemProgram: anchor.web3.SystemProgram.programId,
        })
        .remainingAccounts([
          {
            pubkey: dummy.publicKey,
            isSigner: false,
            isWritable: true,
          },
        ])
        .rpc();

      const actualState = await cacheProgram.account.legacyFeedsConfig.fetch(
        legacyFeedConfigAccount2
      );

      const expectedEntry: ArrayVec<LegacyFeedEntry> = {
        len: new BN(1),
        xs: [
          {
            dataId: Array.from(feedD.dataId),
            legacyFeed: dummy.publicKey,
            writeDisabled: 0,
          },
        ],
      };

      const entryEq = (a: LegacyFeedEntry, b: LegacyFeedEntry) => {
        return (
          a.legacyFeed.equals(b.legacyFeed) &&
          Buffer.from(a.dataId).equals(Buffer.from(b.dataId))
        );
      };

      assert.isTrue(
        arrayVecEquals(expectedEntry, actualState.idToFeed, entryEq),
        "workflow metadata equal"
      );

      assert.isTrue(
        actualState.legacyStore.equals(mockLegacyStoreProgram.programId)
      );
    });

    describe("Update & Close", function () {
      beforeEach(async () => {
        const dummy = Keypair.generate();

        await cacheProgram.methods
          .initLegacyFeedsConfig([feedD.dataId] as any)
          .accounts({
            owner: provider.publicKey,
            state: cacheStateAccount.publicKey,
            legacyStore: mockLegacyStoreProgram.programId,
            legacyFeedsConfig: legacyFeedConfigAccount2,
            systemProgram: anchor.web3.SystemProgram.programId,
          })
          .remainingAccounts([
            {
              pubkey: dummy.publicKey,
              isSigner: false,
              isWritable: true,
            },
          ])
          .rpc();
      });

      it("Update", async () => {
        const sortedDataIds = [feedD.dataId, feedC.dataId].sort((a, b) =>
          a.compare(b)
        );

        const dummy1 = Keypair.generate();
        const dummy2 = Keypair.generate();

        await cacheProgram.methods
          .updateLegacyFeedsConfig(sortedDataIds as any, [false, false])
          .accounts({
            owner: provider.publicKey,
            state: cacheStateAccount.publicKey,
            legacyStore: mockLegacyStoreProgram.programId,
            legacyFeedsConfig: legacyFeedConfigAccount2,
          })
          .remainingAccounts([
            {
              pubkey: dummy1.publicKey,
              isSigner: false,
              isWritable: true,
            },
            {
              pubkey: dummy2.publicKey,
              isSigner: false,
              isWritable: true,
            },
          ])
          .rpc();

        const actualState = await cacheProgram.account.legacyFeedsConfig.fetch(
          legacyFeedConfigAccount2
        );

        const expectedEntry: ArrayVec<LegacyFeedEntry> = {
          len: new BN(2),
          xs: [
            {
              dataId: Array.from(sortedDataIds[0]),
              legacyFeed: dummy1.publicKey,
              writeDisabled: 0,
            },
            {
              dataId: Array.from(sortedDataIds[1]),
              legacyFeed: dummy2.publicKey,
              writeDisabled: 0,
            },
          ],
        };

        const entryEq = (a: LegacyFeedEntry, b: LegacyFeedEntry) => {
          return (
            a.legacyFeed.equals(b.legacyFeed) &&
            Buffer.from(a.dataId).equals(Buffer.from(b.dataId)) &&
            a.writeDisabled == b.writeDisabled
          );
        };

        assert.isTrue(
          arrayVecEquals(expectedEntry, actualState.idToFeed, entryEq),
          "entries equal"
        );

        assert.isTrue(
          actualState.legacyStore.equals(mockLegacyStoreProgram.programId)
        );
      });

      it("Close", async () => {
        await cacheProgram.methods
          .closeLegacyFeedsConfig()
          .accounts({
            owner: provider.publicKey,
            state: cacheStateAccount.publicKey,
            legacyFeedsConfig: legacyFeedConfigAccount2,
          })
          .rpc();

        try {
          await cacheProgram.account.writePermissionFlag.fetch(
            legacyFeedConfigAccount2
          );
          assert.fail("Account should not exist anymore");
        } catch (err) {
          if (!err.message.includes("Account does not exist")) {
            assert.fail("Account should not exist anymore");
          }
        }

        // assert.isRejected(
        //   cacheProgram.account.writePermissionFlag.fetch(
        //     legacyFeedConfigAccount2
        //   ),
        //   /Account does not exist/
        // );
      });
    });
  });

  describe("Initialize Data Feed Reports", function () {
    let cacheStateAccount: Keypair;

    // todo: add more tests --
    // b. if you pass in the wrong length
    // c. if feed admin is not authorized
    // d. out of order data ids and out of order remaining accounts

    beforeEach(async () => {
      cacheStateAccount = Keypair.generate();

      await cacheProgram.methods
        .initialize([feedAdminA.provider.publicKey])
        .accounts({
          state: cacheStateAccount.publicKey,
          owner: provider.publicKey,
          forwarderProgram: forwarderProgram.programId,
          systemProgram: anchor.web3.SystemProgram.programId,
        })
        .signers([cacheStateAccount])
        .rpc();
    });

    it("Initialize data feed reports", async () => {
      const feedAReportPDA = decimalReportPDA(
        cacheStateAccount.publicKey,
        feedA.dataId
      );
      const feedBReportPDA = decimalReportPDA(
        cacheStateAccount.publicKey,
        feedB.dataId
      );
      const feedCReportPDA = decimalReportPDA(
        cacheStateAccount.publicKey,
        feedC.dataId
      );

      await sendLamports(
        defaultConnection,
        feedAReportPDA,
        3 * LAMPORTS_PER_SOL
      );

      // test initialization
      await cacheProgram.methods
        .initDecimalReports([feedA.dataId] as any)
        .accounts({
          feedAdmin: feedAdminA.provider.publicKey,
          state: cacheStateAccount.publicKey,
          systemProgram: anchor.web3.SystemProgram.programId,
        })
        .remainingAccounts([
          {
            pubkey: feedAReportPDA,
            isSigner: false,
            isWritable: true,
          },
        ])
        .signers([feedAdminA.keypair])
        .rpc();

      let feedAState = await cacheProgram.account.decimalReport.fetch(
        feedAReportPDA
      );
      assert.equal(feedAState.timestamp, 0, "timestamp 0");

      // test initialization with existing feed as well
      await cacheProgram.methods
        .initDecimalReports([feedA.dataId, feedB.dataId, feedC.dataId] as any)
        .accounts({
          feedAdmin: feedAdminA.provider.publicKey,
          state: cacheStateAccount.publicKey,
          systemProgram: anchor.web3.SystemProgram.programId,
        })
        .remainingAccounts([
          {
            pubkey: feedAReportPDA,
            isSigner: false,
            isWritable: true,
          },
          {
            pubkey: feedBReportPDA,
            isSigner: false,
            isWritable: true,
          },
          {
            pubkey: feedCReportPDA,
            isSigner: false,
            isWritable: true,
          },
        ])
        .signers([feedAdminA.keypair])
        .rpc();

      feedAState = await cacheProgram.account.decimalReport.fetch(
        feedAReportPDA
      );
      assert.equal(feedAState.timestamp, 0, "timestamp 0");

      const feedBState = await cacheProgram.account.decimalReport.fetch(
        feedBReportPDA
      );
      assert.equal(feedBState.timestamp, 0, "timestamp 0");

      const feedCState = await cacheProgram.account.decimalReport.fetch(
        feedCReportPDA
      );
      assert.equal(feedCState.timestamp, 0, "timestamp 0");
    });

    it("Close decimal feed reports", async () => {
      const feedAReportPDA = decimalReportPDA(
        cacheStateAccount.publicKey,
        feedA.dataId
      );
      const feedAConfig = feedConfigPDA(
        cacheStateAccount.publicKey,
        feedA.dataId
      );

      const feedBReportPDA = decimalReportPDA(
        cacheStateAccount.publicKey,
        feedB.dataId
      );
      const feedBConfig = feedConfigPDA(
        cacheStateAccount.publicKey,
        feedB.dataId
      );

      const feedCReportPDA = decimalReportPDA(
        cacheStateAccount.publicKey,
        feedC.dataId
      );
      const feedCConfig = feedConfigPDA(
        cacheStateAccount.publicKey,
        feedC.dataId
      );

      // test initialization with existing feed as well
      await cacheProgram.methods
        .initDecimalReports([feedA.dataId, feedB.dataId, feedC.dataId] as any)
        .accounts({
          feedAdmin: feedAdminA.provider.publicKey,
          state: cacheStateAccount.publicKey,
          systemProgram: anchor.web3.SystemProgram.programId,
        })
        .remainingAccounts([
          {
            pubkey: feedAReportPDA,
            isSigner: false,
            isWritable: true,
          },
          {
            pubkey: feedBReportPDA,
            isSigner: false,
            isWritable: true,
          },
          {
            pubkey: feedCReportPDA,
            isSigner: false,
            isWritable: true,
          },
        ])
        .signers([feedAdminA.keypair])
        .rpc();

      await cacheProgram.account.decimalReport.fetch(feedAReportPDA);
      await cacheProgram.account.decimalReport.fetch(feedBReportPDA);
      await cacheProgram.account.decimalReport.fetch(feedCReportPDA);

      await cacheProgram.methods
        .setDecimalFeedConfigs(
          [feedA.dataId, feedB.dataId, feedC.dataId] as any,
          [Buffer.alloc(32), Buffer.alloc(32), Buffer.alloc(32)] as any,
          []
        )
        .accounts({
          feedAdmin: feedAdminA.provider.publicKey,
          state: cacheStateAccount.publicKey,
          systemProgram: anchor.web3.SystemProgram.programId,
        })
        .remainingAccounts([
          {
            pubkey: feedAConfig,
            isSigner: false,
            isWritable: true,
          },
          {
            pubkey: feedBConfig,
            isSigner: false,
            isWritable: true,
          },
          {
            pubkey: feedCConfig,
            isSigner: false,
            isWritable: true,
          },
        ])
        .signers([feedAdminA.keypair])
        .rpc();

      await cacheProgram.methods
        .closeDecimalReport(feedA.dataId as any)
        .accounts({
          feedAdmin: feedAdminA.keypair.publicKey,
          state: cacheStateAccount.publicKey,
          decimalReport: feedAReportPDA,
          feedConfig: feedAConfig,
        })
        .signers([feedAdminA.keypair])
        .rpc();

      await cacheProgram.methods
        .closeDecimalReport(feedB.dataId as any)
        .accounts({
          feedAdmin: feedAdminA.keypair.publicKey,
          state: cacheStateAccount.publicKey,
          decimalReport: feedBReportPDA,
          feedConfig: feedBConfig,
        })
        .signers([feedAdminA.keypair])
        .rpc();

      for (const reportAccount of [feedAReportPDA, feedBReportPDA]) {
        try {
          await cacheProgram.account.decimalReport.fetch(reportAccount);
          assert.fail("Account should not exist anymore");
        } catch (err) {
          if (!err.message.includes("Account does not exist")) {
            assert.fail("Account should not exist anymore");
          }
        }
      }
      for (const configAccount of [feedAConfig, feedBConfig]) {
        try {
          await cacheProgram.account.feedConfig.fetch(configAccount);
          assert.fail("Account should not exist anymore");
        } catch (err) {
          if (!err.message.includes("Account does not exist")) {
            assert.fail("Account should not exist anymore");
          }
        }
      }

      await cacheProgram.account.decimalReport.fetch(feedCReportPDA);
      await cacheProgram.account.feedConfig.fetch(feedCConfig);
    });
  });

  describe("Decimal Feed Configuration", function () {
    let cacheStateAccount: Keypair;

    beforeEach(async () => {
      cacheStateAccount = Keypair.generate();

      await cacheProgram.methods
        .initialize([feedAdminA.provider.publicKey])
        .accounts({
          state: cacheStateAccount.publicKey,
          owner: provider.publicKey,
          forwarderProgram: forwarderProgram.programId,
          systemProgram: anchor.web3.SystemProgram.programId,
        })
        .signers([cacheStateAccount])
        .rpc();
    });

    it("Get Feed Metadata -- Simple", async () => {
      // set feed A's config

      const dummyAllowedSender = Keypair.generate();

      // const workflowMetadata = randomWorkflowMetadata(reportSender.provider.publicKey);

      const workflowMetadata = randomWorkflowMetadata(
        dummyAllowedSender.publicKey
      );

      const reportHash = getReportHash(
        feedA.dataId,
        workflowMetadata.allowedSender.toBuffer(),
        workflowMetadata.allowedWorkflowOwner,
        workflowMetadata.allowedWorkflowName
      );

      // find the PDAs

      const feedAConfigAccount = feedConfigPDA(
        cacheStateAccount.publicKey,
        feedA.dataId
      );
      const feedAPermissionFlagAccount = permissionFlagPDA(
        cacheStateAccount.publicKey,
        reportHash
      );

      await cacheProgram.methods
        .setDecimalFeedConfigs(
          [feedA.dataId] as any,
          [feedA.description] as any,
          [workflowMetadata]
        )
        .accounts({
          feedAdmin: feedAdminA.provider.publicKey,
          state: cacheStateAccount.publicKey,
          systemProgram: anchor.web3.SystemProgram.programId,
        })
        .remainingAccounts([
          {
            pubkey: feedAConfigAccount,
            isSigner: false,
            isWritable: true,
          },
          {
            pubkey: feedAPermissionFlagAccount,
            isSigner: false,
            isWritable: true,
          },
        ])
        .signers([feedAdminA.keypair])
        .rpc();

      const simulateResponse = await cacheProgram.methods
        .queryFeedMetadata(feedA.dataId as any, new BN(0), new BN(0))
        .accounts({
          cacheState: cacheStateAccount.publicKey,
          feedConfig: feedAConfigAccount,
        })
        .signers([])
        .simulate();

      const [_ixDiscrimiantor, base64Data] = parseReturnData(
        simulateResponse.raw
      );

      const WorkflowMetadataLayout = struct([
        publicKey("allowed_sender"),
        array(u8(), 20, "allowed_workflow_owner"),
        array(u8(), 10, "allowed_workflow_name"),
      ]);

      const WorkflowMetadataVecLayout = vec(WorkflowMetadataLayout);

      const decodedMetadatas = WorkflowMetadataVecLayout.decode(
        Buffer.from(base64Data, "base64")
      ) as Array<{
        allowed_sender: PublicKey;
        allowed_workflow_owner: Uint8Array;
        allowed_workflow_name: Uint8Array;
      }>;

      assert.equal(decodedMetadatas.length, 1, "one workflow found");
      assert.isTrue(
        decodedMetadatas[0].allowed_sender.equals(
          workflowMetadata.allowedSender
        )
      );
      assert.isTrue(
        Buffer.from(decodedMetadatas[0].allowed_workflow_name).equals(
          Buffer.from(workflowMetadata.allowedWorkflowName)
        )
      );
      assert.isTrue(
        Buffer.from(decodedMetadatas[0].allowed_workflow_owner).equals(
          Buffer.from(workflowMetadata.allowedWorkflowOwner)
        )
      );
    });

    it("Simple Set Feed Configs", async () => {
      // set feed A's config

      const dummyAllowedSender = Keypair.generate();

      // const workflowMetadata = randomWorkflowMetadata(reportSender.provider.publicKey);

      const workflowMetadata = randomWorkflowMetadata(
        dummyAllowedSender.publicKey
      );

      const reportHash = getReportHash(
        feedA.dataId,
        workflowMetadata.allowedSender.toBuffer(),
        workflowMetadata.allowedWorkflowOwner,
        workflowMetadata.allowedWorkflowName
      );

      // find the PDAs

      const feedAConfigAccount = feedConfigPDA(
        cacheStateAccount.publicKey,
        feedA.dataId
      );
      const feedAPermissionFlagAccount = permissionFlagPDA(
        cacheStateAccount.publicKey,
        reportHash
      );

      await cacheProgram.methods
        .setDecimalFeedConfigs(
          [feedA.dataId] as any,
          [feedA.description] as any,
          [workflowMetadata]
        )
        .accounts({
          feedAdmin: feedAdminA.provider.publicKey,
          state: cacheStateAccount.publicKey,
          systemProgram: anchor.web3.SystemProgram.programId,
        })
        .remainingAccounts([
          {
            pubkey: feedAConfigAccount,
            isSigner: false,
            isWritable: true,
          },
          {
            pubkey: feedAPermissionFlagAccount,
            isSigner: false,
            isWritable: true,
          },
        ])
        .signers([feedAdminA.keypair])
        .rpc();

      const actualWritePermissionFlag =
        await cacheProgram.account.writePermissionFlag.fetch(
          feedAPermissionFlagAccount
        );

      assert.isTrue(
        Object.keys(actualWritePermissionFlag).length === 0,
        "flag exists"
      );

      const actualFeedConfig = await cacheProgram.account.feedConfig.fetch(
        feedAConfigAccount
      );

      assert.isTrue(
        Buffer.from(actualFeedConfig.description).equals(feedA.description),
        "descriptions equal"
      );

      const expectedWorkflowMetadas: ArrayVec<WorkflowMetadata> = {
        len: new BN(1),
        xs: [workflowMetadata].map((x) => ({
          allowedSender: x.allowedSender,
          allowedWorkflowOwner: Array.from(x.allowedWorkflowOwner),
          allowedWorkflowName: Array.from(x.allowedWorkflowName),
        })),
      };

      assert.isTrue(
        arrayVecEquals(
          expectedWorkflowMetadas,
          actualFeedConfig.workflowMetadata,
          workflowMetadataEq
        ),
        "workflow metadata equal"
      );
    });

    it("Set Feed Config + Preview Feed Config", async () => {
      // 1. set config
      // 2. preview changes from config update
      // 3. enact config update

      const initialDataIds = [feedA.dataId, feedB.dataId];

      const initialDescriptions = [randomDescription(), feedB.description]; // change assetA's description while we're at it!

      const initialWorkflowMetadatas = Array.from({ length: 3 }).map(() => {
        return randomWorkflowMetadata(reportSender.provider.publicKey);
      });

      const initialFeedConfigAccounts = initialDataIds.map((id) =>
        feedConfigPDA(cacheStateAccount.publicKey, id)
      );

      const initialReportHashes: Buffer[] = initialDataIds.flatMap((dataId) =>
        initialWorkflowMetadatas.map((metadata) =>
          getReportHash(
            dataId,
            metadata.allowedSender.toBuffer(),
            metadata.allowedWorkflowOwner,
            metadata.allowedWorkflowName
          )
        )
      );

      const initialPermissionFlagAccounts = initialReportHashes.map((hash) =>
        permissionFlagPDA(cacheStateAccount.publicKey, hash)
      );

      const initialRemainingAccounts = initialFeedConfigAccounts
        .map((acc) => ({ pubkey: acc, isSigner: false, isWritable: true }))
        .concat(
          initialPermissionFlagAccounts.map((acc) => ({
            pubkey: acc,
            isSigner: false,
            isWritable: true,
          }))
        );

      // anchor discriminator size + feed config size
      const rentExemptLamports =
        await defaultConnection.getMinimumBalanceForRentExemption(8 + 1032);
      const sentAmountLamports = rentExemptLamports - 100;

      // send under rent exemption amount to check if remaining rent is covered by signer
      await sendLamports(
        defaultConnection,
        initialFeedConfigAccounts[0],
        sentAmountLamports
      );

      assert.equal(
        await defaultConnection.getBalance(initialFeedConfigAccounts[0]),
        sentAmountLamports,
        "expected airdropped amount"
      );

      await sendLamports(
        defaultConnection,
        initialPermissionFlagAccounts[0],
        1 * LAMPORTS_PER_SOL
      );

      await cacheProgram.methods
        .setDecimalFeedConfigs(
          initialDataIds as any,
          initialDescriptions as any,
          initialWorkflowMetadatas
        )
        .accounts({
          feedAdmin: feedAdminA.provider.publicKey,
          state: cacheStateAccount.publicKey,
          systemProgram: anchor.web3.SystemProgram.programId,
        })
        .remainingAccounts(initialRemainingAccounts)
        .signers([feedAdminA.keypair])
        .rpc();

      // check remaining rent has been paid
      assert.equal(
        await defaultConnection.getBalance(initialFeedConfigAccounts[0]),
        rentExemptLamports,
        "rent exempt amount"
      );

      // 2. preview changes
      const dataIds2 = [feedA.dataId, feedB.dataId];

      const descriptions2 = [randomDescription(), feedB.description]; // change assetA's description while we're at it!

      const workflowMetadatas2 = Array.from({ length: 4 }).map(() => {
        return randomWorkflowMetadata(reportSender.provider.publicKey);
      });

      const feedConfigAccounts2 = dataIds2.map((id) =>
        feedConfigPDA(cacheStateAccount.publicKey, id)
      );

      const reportHashes2: Buffer[] = dataIds2.flatMap((dataId) =>
        workflowMetadatas2.map((metadata) =>
          getReportHash(
            dataId,
            metadata.allowedSender.toBuffer(),
            metadata.allowedWorkflowOwner,
            metadata.allowedWorkflowName
          )
        )
      );

      const permissionFlagAccounts2 = reportHashes2.map((hash) =>
        permissionFlagPDA(cacheStateAccount.publicKey, hash)
      );

      const remainingAccounts2 = feedConfigAccounts2
        .map((acc) => ({ pubkey: acc, isSigner: false, isWritable: true }))
        .concat(
          permissionFlagAccounts2.map((acc) => ({
            pubkey: acc,
            isSigner: false,
            isWritable: true,
          }))
        );

      const simulateResponse = await cacheProgram.methods
        .previewDecimalFeedConfigs(
          dataIds2 as any,
          descriptions2 as any,
          workflowMetadatas2
        )
        .accounts({
          state: cacheStateAccount.publicKey,
        })
        .remainingAccounts(remainingAccounts2)
        .signers([])
        .simulate();

      console.log(simulateResponse);

      const [_ixDiscrimiantor, base64Data] = parseReturnData(
        simulateResponse.raw
      );

      const AccountsVecLayout = vec(publicKey());

      const deletePermissionAccounts = AccountsVecLayout.decode(
        Buffer.from(base64Data, "base64")
      ) as Array<PublicKey>;

      assert.equal(deletePermissionAccounts.length, 6); // 6 workflow permissions are to be invalidated

      const array1 = deletePermissionAccounts.map((pk) => pk.toBase58());
      const array2 = initialPermissionFlagAccounts.map((pk) => pk.toBase58()); // old permission flag accounts

      expect(array1).to.have.deep.members(array2);

      // lastly, do set config
      await cacheProgram.methods
        .setDecimalFeedConfigs(
          dataIds2 as any,
          descriptions2 as any,
          workflowMetadatas2
        )
        .accounts({
          feedAdmin: feedAdminA.keypair.publicKey,
          state: cacheStateAccount.publicKey,
        })
        .remainingAccounts(
          remainingAccounts2.concat(
            deletePermissionAccounts.map((acc) => ({
              pubkey: acc,
              isSigner: false,
              isWritable: true,
            }))
          )
        )
        .signers([feedAdminA.keypair])
        .rpc();

      for (let x of deletePermissionAccounts) {
        try {
          await cacheProgram.account.writePermissionFlag.fetch(x);
          assert.fail("Account should not exist anymore");
        } catch (err) {
          if (!err.message.includes("Account does not exist")) {
            assert.fail("Account should not exist anymore");
          }
        }
      }

      // check the state of stuff here

      const expectedWorkflowMetadas2: ArrayVec<WorkflowMetadata> = {
        len: new BN(4),
        xs: workflowMetadatas2.map((x) => ({
          allowedSender: x.allowedSender,
          allowedWorkflowOwner: Array.from(x.allowedWorkflowOwner),
          allowedWorkflowName: Array.from(x.allowedWorkflowName),
        })),
      };

      for (let i = 0; i < dataIds2.length; i++) {
        const actualFeedConfigAsset =
          await cacheProgram.account.feedConfig.fetch(feedConfigAccounts2[i]);
        assert.isTrue(
          Buffer.from(actualFeedConfigAsset.description).equals(
            descriptions2[i]
          ),
          "descriptions equal"
        );
        assert.isTrue(
          arrayVecEquals(
            expectedWorkflowMetadas2,
            actualFeedConfigAsset.workflowMetadata,
            workflowMetadataEq
          ),
          "workflow metadata equal"
        );
      }
    });
  });

  describe("Forwarder calls `on_report`", function () {
    let cacheStateAccount: Keypair;

    let forwarder: Forwarder;
    let forwarderAuthority: anchor.web3.PublicKey;

    let legacyFeeds: Feed[];

    let legacyWorkflowMetadatas: {
      allowedSender: anchor.web3.PublicKey;
      allowedWorkflowOwner: Buffer;
      allowedWorkflowName: Buffer;
    }[];

    let legacyReportHashes: Buffer[];

    let legacyFeedConfigAccount: PublicKey;

    let dummyLegacyFeedAccounts: Keypair[];
    let dummyLegacyFeedAccountMetas: AccountMeta[];

    beforeEach(async () => {
      // set up forwarder
      forwarder = new Forwarder(forwarderProgram, provider)
        .withState(Keypair.generate())
        .withOracles(1, 12, 41);

      await forwarder.initialize();
      await forwarder.initOraclesConfig();

      let [authority, _] = calculateForwarderAuthorityBump(
        forwarder.state.publicKey,
        cacheProgram.programId,
        forwarder.forwarderProgram.programId
      );
      forwarderAuthority = authority;

      cacheStateAccount = Keypair.generate();
      legacyFeedConfigAccount = legacyFeedsConfigPDA(
        cacheStateAccount.publicKey
      );
      dummyLegacyFeedAccounts = Array.from({ length: 2 }, () =>
        Keypair.generate()
      );
      dummyLegacyFeedAccountMetas = dummyLegacyFeedAccounts.map((k) => ({
        pubkey: k.publicKey,
        isSigner: false,
        isWritable: true,
      }));
      legacyFeeds = [feedA, feedB].sort((a, b) => a.dataId.compare(b.dataId));
      legacyWorkflowMetadatas = newWorkflows(1, forwarderAuthority);
      legacyReportHashes = legacyFeeds.map((f) => {
        return getReportHash(
          f.dataId,
          legacyWorkflowMetadatas[0].allowedSender.toBuffer(),
          legacyWorkflowMetadatas[0].allowedWorkflowOwner,
          legacyWorkflowMetadatas[0].allowedWorkflowName
        );
      });

      // setup cache: we have to initialize it, configure legacy configs, and configure decimal reports,

      // init cache

      await cacheProgram.methods
        .initialize([feedAdminA.provider.publicKey])
        .accounts({
          state: cacheStateAccount.publicKey,
          owner: provider.publicKey,
          forwarderProgram: forwarderProgram.programId,
          systemProgram: anchor.web3.SystemProgram.programId,
        })
        .signers([cacheStateAccount])
        .rpc();

      // init legacy feed configs

      await cacheProgram.methods
        .initLegacyFeedsConfig(legacyFeeds.map((f) => f.dataId) as any)
        .accounts({
          owner: provider.publicKey,
          state: cacheStateAccount.publicKey,
          legacyStore: mockLegacyStoreProgram.programId,
          legacyFeedsConfig: legacyFeedConfigAccount,
          systemProgram: anchor.web3.SystemProgram.programId,
        })
        .remainingAccounts(dummyLegacyFeedAccountMetas)
        .rpc();

      // set decimal configs

      const remainingAccounts = legacyFeeds
        .map((f) => ({
          pubkey: feedConfigPDA(cacheStateAccount.publicKey, f.dataId),
          isSigner: false,
          isWritable: true,
        }))
        .concat(
          legacyReportHashes.map((r) => ({
            pubkey: permissionFlagPDA(cacheStateAccount.publicKey, r),
            isSigner: false,
            isWritable: true,
          }))
        );

      await cacheProgram.methods
        .setDecimalFeedConfigs(
          legacyFeeds.map((f) => f.dataId) as any,
          legacyFeeds.map((f) => f.description) as any,
          legacyWorkflowMetadatas
        )
        .accounts({
          feedAdmin: feedAdminA.provider.publicKey,
          state: cacheStateAccount.publicKey,
          systemProgram: anchor.web3.SystemProgram.programId,
        })
        .remainingAccounts(remainingAccounts)
        .signers([feedAdminA.keypair])
        .rpc();

      // init decimal reports

      await cacheProgram.methods
        .initDecimalReports([feedA.dataId, feedB.dataId] as any)
        .accounts({
          feedAdmin: feedAdminA.provider.publicKey,
          state: cacheStateAccount.publicKey,
          systemProgram: anchor.web3.SystemProgram.programId,
        })
        .remainingAccounts([
          {
            pubkey: decimalReportPDA(cacheStateAccount.publicKey, feedA.dataId),
            isSigner: false,
            isWritable: true,
          },
          {
            pubkey: decimalReportPDA(cacheStateAccount.publicKey, feedB.dataId),
            isSigner: false,
            isWritable: true,
          },
        ])
        .signers([feedAdminA.keypair])
        .rpc();
    });

    it("Updates feed A and feed B with legacy write ", async () => {
      // sorted legacy feed accounts need to be passed into remaining account context
      const sortedDummyLegacyFeedAccountMetas = [
        ...dummyLegacyFeedAccountMetas,
      ].sort((a, b) => a.pubkey.toBuffer().compare(b.pubkey.toBuffer()));

      const submissionEvent = waitForEvent(
        mockLegacyStoreProgram,
        "Submit",
        (event: any, slot) => {
          assert.strictEqual(
            event.feeds.length,
            dummyLegacyFeedAccountMetas.length
          );
          event.feeds.forEach((k, i) => {
            assert.ok(
              k.equals(dummyLegacyFeedAccountMetas[i].pubkey),
              `Mismatch at index ${i}: got ${k.toBase58()}, expected ${dummyLegacyFeedAccountMetas[
                i
              ].pubkey.toBase58()}`
            );
          });

          assert.equal(event.rounds[0].timestamp, 10);
          assert.isTrue(new BN(10).eq(event.rounds[0].answer));
          assert.equal(event.rounds[1].timestamp, 11);
          assert.isTrue(new BN(11).eq(event.rounds[1].answer));
        }
      );

      const feedReports = legacyFeeds.map((f, i) => {
        return cacheProgram.coder.types.encode("ReceivedDecimalReport", {
          timestamp: new BN(i + 10),
          answer: new BN(i + 10),
          dataId: f.dataId,
        });
      });
      const lenPrefix = Buffer.alloc(4);
      lenPrefix.writeUInt32LE(2, 0);
      const fullEncodedVec = Buffer.concat([lenPrefix].concat(feedReports));

      await forwarder.report(
        cacheProgram.programId,
        fullEncodedVec,
        legacyWorkflowMetadatas[0].allowedWorkflowName,
        legacyWorkflowMetadatas[0].allowedWorkflowOwner,
        [
          {
            pubkey: cacheStateAccount.publicKey,
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: mockLegacyStoreProgram.programId, // legacy store
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: legacyFeedConfigAccount, // legacy feed config
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: legacyWriterPDA(cacheStateAccount.publicKey), // legacy writer
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: decimalReportPDA(
              cacheStateAccount.publicKey,
              legacyFeeds[0].dataId
            ),
            isSigner: false,
            isWritable: true,
          },
          {
            pubkey: decimalReportPDA(
              cacheStateAccount.publicKey,
              legacyFeeds[1].dataId
            ),
            isSigner: false,
            isWritable: true,
          },
          {
            pubkey: permissionFlagPDA(
              cacheStateAccount.publicKey,
              legacyReportHashes[0]
            ),
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: permissionFlagPDA(
              cacheStateAccount.publicKey,
              legacyReportHashes[1]
            ),
            isSigner: false,
            isWritable: false,
          },
          sortedDummyLegacyFeedAccountMetas[0],
          sortedDummyLegacyFeedAccountMetas[1],
        ]
      );

      const report1 = await cacheProgram.account.decimalReport.fetch(
        decimalReportPDA(cacheStateAccount.publicKey, legacyFeeds[0].dataId)
      );
      assert.isTrue(report1.answer.eq(new BN(10)), "answers match");
      assert.isTrue(report1.timestamp == 10, "answers match");

      const report2 = await cacheProgram.account.decimalReport.fetch(
        decimalReportPDA(cacheStateAccount.publicKey, legacyFeeds[1].dataId)
      );
      assert.isTrue(report2.answer.eq(new BN(11)), "answers match");
      assert.isTrue(report2.timestamp == 11, "answers match");

      await submissionEvent;
    });

    it("Update feed A without legacy write + check query method", async () => {
      // first we must update the workflow metadata to work with the desired receiver

      const reportHash = getReportHash(
        feedA.dataId,
        legacyWorkflowMetadatas[0].allowedSender.toBuffer(),
        legacyWorkflowMetadatas[0].allowedWorkflowOwner,
        legacyWorkflowMetadatas[0].allowedWorkflowName
      );

      const singleReport = cacheProgram.coder.types.encode(
        "ReceivedDecimalReport",
        {
          timestamp: new BN(123),
          answer: new BN(321),
          dataId: feedA.dataId,
        }
      );

      const lenPrefix = Buffer.alloc(4);
      lenPrefix.writeUInt32LE(1, 0);

      // Step 3: Concatenate length + all reports
      const fullEncodedVec = Buffer.concat([lenPrefix, singleReport]);

      const feedAReportPDA = decimalReportPDA(
        cacheStateAccount.publicKey,
        feedA.dataId
      );
      const permissionFlagAPDA = permissionFlagPDA(
        cacheStateAccount.publicKey,
        reportHash
      );

      // make it work with the right report hash permissions
      await forwarder.report(
        cacheProgram.programId,
        fullEncodedVec,
        legacyWorkflowMetadatas[0].allowedWorkflowName,
        legacyWorkflowMetadatas[0].allowedWorkflowOwner,
        [
          {
            pubkey: cacheStateAccount.publicKey,
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: cacheProgram.programId, // legacy store (omitted)
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: cacheProgram.programId, // legacy feed config (omitted)
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: cacheProgram.programId, // legacy writer (omitted)
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: feedAReportPDA,
            isSigner: false,
            isWritable: true,
          },
          {
            pubkey: permissionFlagAPDA,
            isSigner: false,
            isWritable: true,
          },
        ]
      );

      const report1 = await cacheProgram.account.decimalReport.fetch(
        feedAReportPDA
      );
      assert.isTrue(report1.answer.eq(new BN(321)), "answers match");
      assert.isTrue(report1.timestamp == 123, "answers match");

      const simulateResponse = await cacheProgram.methods
        .queryValues([feedA.dataId] as any)
        .accounts({
          cacheState: cacheStateAccount.publicKey,
        })
        .remainingAccounts([
          {
            pubkey: feedAReportPDA,
            isSigner: false,
            isWritable: false,
          },
        ])
        .signers([])
        .simulate();

      const [_ixDiscrimiantor, base64Data] = parseReturnData(
        simulateResponse.raw
      );

      const DecimalReportLayout = struct([u32("timestamp"), u128("answer")]);
      const DecimalReportVecLayout = vec(DecimalReportLayout);

      const decodedReportValues = DecimalReportVecLayout.decode(
        Buffer.from(base64Data, "base64")
      ) as Array<{ timestamp: number; answer: BN }>;

      assert.equal(decodedReportValues.length, 1, "one report expected");
      assert.equal(decodedReportValues[0].timestamp, 123, "same timestamp");
      assert.isTrue(
        new BN(321).eq(decodedReportValues[0].answer),
        "same answer"
      );
    });

    it("Attempt feed A stale update", async () => {
      const reportHash = getReportHash(
        feedA.dataId,
        legacyWorkflowMetadatas[0].allowedSender.toBuffer(),
        legacyWorkflowMetadatas[0].allowedWorkflowOwner,
        legacyWorkflowMetadatas[0].allowedWorkflowName
      );

      const singleReport = cacheProgram.coder.types.encode(
        "ReceivedDecimalReport",
        {
          timestamp: new BN(123),
          answer: new BN(321),
          dataId: feedA.dataId,
        }
      );

      const lenPrefix = Buffer.alloc(4);
      lenPrefix.writeUInt32LE(1, 0);

      // Step 3: Concatenate length + all reports
      const fullEncodedVec = Buffer.concat([lenPrefix, singleReport]);

      const feedAReportPDA = decimalReportPDA(
        cacheStateAccount.publicKey,
        feedA.dataId
      );
      const permissionFlagAPDA = permissionFlagPDA(
        cacheStateAccount.publicKey,
        reportHash
      );

      // make it work with the right report hash permissions
      await forwarder.report(
        cacheProgram.programId,
        fullEncodedVec,
        legacyWorkflowMetadatas[0].allowedWorkflowName,
        legacyWorkflowMetadatas[0].allowedWorkflowOwner,
        [
          {
            pubkey: cacheStateAccount.publicKey,
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: cacheProgram.programId, // legacy store (omitted)
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: cacheProgram.programId, // legacy feed config (omitted)
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: cacheProgram.programId, // legacy writer (omitted)
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: feedAReportPDA,
            isSigner: false,
            isWritable: true,
          },
          {
            pubkey: permissionFlagAPDA,
            isSigner: false,
            isWritable: true,
          },
        ]
      );

      const report = await cacheProgram.account.decimalReport.fetch(
        feedAReportPDA
      );
      assert.isTrue(report.answer.eq(new BN(321)), "answers match");
      assert.isTrue(report.timestamp == 123, "answers match");

      const staleReport = cacheProgram.coder.types.encode(
        "ReceivedDecimalReport",
        {
          timestamp: new BN(123),
          answer: new BN(321),
          dataId: feedA.dataId,
        }
      );

      const staleLenPrefix = Buffer.alloc(4);
      staleLenPrefix.writeUInt32LE(1, 0);

      // Step 3: Concatenate length + all reports
      const staleEncodedVec = Buffer.concat([staleLenPrefix, staleReport]);

      const staleReportEvent = waitForEvent(
        cacheProgram,
        "StaleDecimalReport",
        (event: any, slot) => {
          assert.isTrue(
            Buffer.from(event.dataId).equals(feedA.dataId),
            "data ids same"
          );
          assert.equal(event.receivedTimestamp, 123);
          assert.equal(event.latestTimestamp, 123);
        }
      );

      // make it work with the right report hash permissions
      await forwarder.report(
        cacheProgram.programId,
        staleEncodedVec,
        legacyWorkflowMetadatas[0].allowedWorkflowName,
        legacyWorkflowMetadatas[0].allowedWorkflowOwner,
        [
          {
            pubkey: cacheStateAccount.publicKey,
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: cacheProgram.programId, // legacy store (omitted)
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: cacheProgram.programId, // legacy feed config (omitted)
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: cacheProgram.programId, // legacy writer (omitted)
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: feedAReportPDA,
            isSigner: false,
            isWritable: true,
          },
          {
            pubkey: permissionFlagAPDA,
            isSigner: false,
            isWritable: true,
          },
        ]
      );

      const report1 = await cacheProgram.account.decimalReport.fetch(
        feedAReportPDA
      );
      assert.isTrue(report1.answer.eq(new BN(321)), "answers match");
      assert.isTrue(report1.timestamp == 123, "answers match");

      await staleReportEvent;
    });

    it("Attempt feed A without correct permissions", async () => {
      const unauthorizedWorkflow = newWorkflows(1, forwarderAuthority);

      const reportHash = getReportHash(
        feedA.dataId,
        unauthorizedWorkflow[0].allowedSender.toBuffer(),
        unauthorizedWorkflow[0].allowedWorkflowOwner,
        unauthorizedWorkflow[0].allowedWorkflowName
      );

      const singleReport = cacheProgram.coder.types.encode(
        "ReceivedDecimalReport",
        {
          timestamp: new BN(123),
          answer: new BN(321),
          dataId: feedA.dataId,
        }
      );

      const lenPrefix = Buffer.alloc(4);
      lenPrefix.writeUInt32LE(1, 0);

      // Step 3: Concatenate length + all reports
      const fullEncodedVec = Buffer.concat([lenPrefix, singleReport]);

      const feedAReportPDA = decimalReportPDA(
        cacheStateAccount.publicKey,
        feedA.dataId
      );
      const permissionFlagAPDA = permissionFlagPDA(
        cacheStateAccount.publicKey,
        reportHash
      );

      const invalidUpdatePermissionEvent = waitForEvent(
        cacheProgram,
        "InvalidUpdatePermission",
        (event: any, slot) => {
          assert.isTrue(
            Buffer.from(event.dataId).equals(feedA.dataId),
            "expected data id"
          );
          assert.isTrue(
            unauthorizedWorkflow[0].allowedSender.equals(event.sender),
            "expected sender"
          );
          assert.isTrue(
            Buffer.from(event.workflowOwner).equals(
              unauthorizedWorkflow[0].allowedWorkflowOwner
            ),
            "expected owner"
          );
          assert.isTrue(
            Buffer.from(event.workflowName).equals(
              unauthorizedWorkflow[0].allowedWorkflowName
            ),
            "expected name"
          );
        }
      );

      // make it work with the right report hash permissions
      await forwarder.report(
        cacheProgram.programId,
        fullEncodedVec,
        unauthorizedWorkflow[0].allowedWorkflowName,
        unauthorizedWorkflow[0].allowedWorkflowOwner,
        [
          {
            pubkey: cacheStateAccount.publicKey,
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: cacheProgram.programId, // legacy store (omitted)
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: cacheProgram.programId, // legacy feed config (omitted)
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: cacheProgram.programId, // legacy writer (omitted)
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: feedAReportPDA,
            isSigner: false,
            isWritable: true,
          },
          {
            pubkey: permissionFlagAPDA,
            isSigner: false,
            isWritable: true,
          },
        ]
      );

      const report1 = await cacheProgram.account.decimalReport.fetch(
        feedAReportPDA
      );

      assert.isTrue(report1.answer.eq(new BN(0)), "unreported");
      assert.isTrue(report1.timestamp == 0, "unreported");

      await invalidUpdatePermissionEvent;
    });

    it("Attempt feed A 2x", async () => {
      const reportHash = getReportHash(
        feedA.dataId,
        legacyWorkflowMetadatas[0].allowedSender.toBuffer(),
        legacyWorkflowMetadatas[0].allowedWorkflowOwner,
        legacyWorkflowMetadatas[0].allowedWorkflowName
      );

      const singleReport = cacheProgram.coder.types.encode(
        "ReceivedDecimalReport",
        {
          timestamp: new BN(123),
          answer: new BN(321),
          dataId: feedA.dataId,
        }
      );

      const lenPrefix = Buffer.alloc(4);
      lenPrefix.writeUInt32LE(1, 0);

      // Step 3: Concatenate length + all reports
      const fullEncodedVec = Buffer.concat([lenPrefix, singleReport]);

      const feedAReportPDA = decimalReportPDA(
        cacheStateAccount.publicKey,
        feedA.dataId
      );
      const permissionFlagAPDA = permissionFlagPDA(
        cacheStateAccount.publicKey,
        reportHash
      );

      // make it work with the right report hash permissions
      await forwarder.report(
        cacheProgram.programId,
        fullEncodedVec,
        legacyWorkflowMetadatas[0].allowedWorkflowName,
        legacyWorkflowMetadatas[0].allowedWorkflowOwner,
        [
          {
            pubkey: cacheStateAccount.publicKey,
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: cacheProgram.programId, // legacy store (omitted)
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: cacheProgram.programId, // legacy feed config (omitted)
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: cacheProgram.programId, // legacy writer (omitted)
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: feedAReportPDA,
            isSigner: false,
            isWritable: true,
          },
          {
            pubkey: permissionFlagAPDA,
            isSigner: false,
            isWritable: true,
          },
        ]
      );

      const report = await cacheProgram.account.decimalReport.fetch(
        feedAReportPDA
      );
      assert.isTrue(report.answer.eq(new BN(321)), "answers match");
      assert.isTrue(report.timestamp == 123, "answers match");

      const newReport = cacheProgram.coder.types.encode(
        "ReceivedDecimalReport",
        {
          timestamp: new BN(323),
          answer: new BN(721),
          dataId: feedA.dataId,
        }
      );

      const newLenPrefix = Buffer.alloc(4);
      newLenPrefix.writeUInt32LE(1, 0);

      // Step 3: Concatenate length + all reports
      const newEncodedVec = Buffer.concat([newLenPrefix, newReport]);

      // make it work with the right report hash permissions
      await forwarder.report(
        cacheProgram.programId,
        newEncodedVec,
        legacyWorkflowMetadatas[0].allowedWorkflowName,
        legacyWorkflowMetadatas[0].allowedWorkflowOwner,
        [
          {
            pubkey: cacheStateAccount.publicKey,
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: cacheProgram.programId, // legacy store (omitted)
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: cacheProgram.programId, // legacy feed config (omitted)
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: cacheProgram.programId, // legacy writer (omitted)
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: feedAReportPDA,
            isSigner: false,
            isWritable: true,
          },
          {
            pubkey: permissionFlagAPDA,
            isSigner: false,
            isWritable: true,
          },
        ]
      );

      const report1 = await cacheProgram.account.decimalReport.fetch(
        feedAReportPDA
      );
      assert.isTrue(report1.answer.eq(new BN(721)), "answers match");
      assert.isTrue(report1.timestamp == 323, "answers match");
    });
  });
});

const parseReturnData = (logs: readonly string[]) => {
  const prefix = "Program return: ";
  const returnLog = logs.find((log) => log.startsWith(prefix));
  const log = returnLog.slice(prefix.length);

  // [ixDiscrimiantor, base64Data]
  return log.split(" ", 2);
};
