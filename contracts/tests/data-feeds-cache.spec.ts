import * as anchor from "@coral-xyz/anchor";
import { AnchorProvider, Program, Wallet, getProvider } from "@coral-xyz/anchor";
import { DataFeedsCache } from "../target/types/data_feeds_cache";
import { Keypair, LAMPORTS_PER_SOL, PublicKey } from "@solana/web3.js";
import BN from "bn.js";
import { createHash, randomBytes } from "crypto";
import * as chai from 'chai';
import chaiAsPromised from 'chai-as-promised';
import { assert } from "chai";

import { Forwarder } from "./utils";
import { KeystoneForwarder } from "../target/types/keystone_forwarder";
import { before } from "mocha";
import { DummyReceiver } from "../target/types/dummy_receiver";

chai.use(chaiAsPromised);

type Signer = {
  provider: AnchorProvider,
  keypair: Keypair
};

const newSigner = async (conn: anchor.web3.Connection): Promise<Signer> => {
    // Generate a new keypair
    const keypair = Keypair.generate();
    
    // create provider
    const wallet = new Wallet(keypair);
    const provider = new AnchorProvider(conn, wallet, {});

    // fund account
    const signature = await conn.requestAirdrop(
        keypair.publicKey,
        100 * LAMPORTS_PER_SOL // 100 SOL
    );

    const latestBlockhash = await conn.getLatestBlockhash();

    await conn.confirmTransaction({
        signature,
        ...latestBlockhash
    });

    return { provider, keypair };
}

const newSigners = async (conn: anchor.web3.Connection, n: number) => {
    return await Promise.all(
      Array.from({ length: n }).map(() => newSigner(conn))
    );
}

type ArrayVec<T> = {
  len: BN, // a bignumber,
  xs: Array<T>
}

type EqualsFn<T> = (a: T, b: T) => boolean;

type WorkflowMetadata = {
  allowedSender: PublicKey, 
  allowedWorkflowOwner: number[], 
  allowedWorkflowName: number[]
}

type LegacyFeedEntry = {
  dataId: number[],
  legacyFeed: PublicKey,
  writeDisabled: number
}

// If expected array may be of smaller length than actual array
// We don't care about the rest of the entries since this is an arrayvec!() on-chain
function arrayVecEquals<T>(expected: ArrayVec<T>, actual: ArrayVec<T>, equalsFn: EqualsFn<T> ) {
  return expected.len.eq(actual.len) && expected.xs.reduce((equalsAcc, curr, index) => {
    return equalsAcc && equalsFn(curr, actual.xs[index]);
  }, true)
}

function getReportHash(dataId: Buffer, sender: Buffer, owner: Buffer, name: Buffer) {
    return createHash("sha256")
      .update(Buffer.concat([dataId, sender, owner, name]))
      .digest()
}

function generateDescription(length: number): string {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789';
  let result = '';
  for (let i = 0; i < length; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return result;
}

const randomDescription = () => {
  // const input = Buffer.from("to the moon!", "utf8");
  // const paddedInput = Buffer.alloc(32);      // create 32-byte buffer filled with zeros
  // input.copy(paddedInput);

  const input = Buffer.from(generateDescription(Math.random()*32), "utf8"); // variable length, random string
  const description = Buffer.alloc(32); // rest of buffer is filled with zeros
  input.copy(description);
  return description;
}

const newFeeds = (n: number) => {
  return Array.from({ length: n }).map(() => newFeed());
}

type Feed = {
  dataId: Buffer,
  description: Buffer
}


const newFeed = () => {
  return {
    dataId: randomBytes(16), 
    description: randomDescription()
  };
}; 

const randomWorkflowMetadata = (allowedSender: PublicKey) => {
  return {
    allowedSender: allowedSender, // todo: replace with something else
    allowedWorkflowOwner: randomBytes(20),
    allowedWorkflowName: randomBytes(10),
  }
}

const newWorkflows = (n: number, allowedSender: PublicKey) => {
  return Array.from({length: n}).map(() => {
    return randomWorkflowMetadata(allowedSender)
  });
}

describe("data feeds cache", function () {
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
  let feedELegacy: Feed;
  let feedFLegacy: Feed;
  let otherFeeds: Array<Feed>;

  
  const defaultCacheState = Keypair.generate();

  const program = anchor.workspace.DataFeedsCache as Program<DataFeedsCache>;

  const forwarderProgram = anchor.workspace.KeystoneForwarder as Program<KeystoneForwarder>;


  const mockLegacyStore = anchor.workspace.DummyReceiver as Program<DummyReceiver>;



  before(async () => {
    [feedAdminA, reportSender, ...otherSigners] = await newSigners(defaultConnection, 5);
    [feedA, feedB, feedC, feedD, feedELegacy, feedFLegacy, ...otherFeeds] = newFeeds(10);
  });

  const feedConfigPDA = (dataId: Buffer) => {
    const [pda, _bump] = PublicKey.findProgramAddressSync(
      [
        Buffer.from(anchor.utils.bytes.utf8.encode("feed_config")),
        defaultCacheState.publicKey.toBuffer(),
        dataId,
      ],
      program.programId
    );
    return pda;
  }

  const permissionFlagPDA = (reportHash: Buffer) => {
    const [pda, _bump] = PublicKey.findProgramAddressSync(
      [
        Buffer.from(anchor.utils.bytes.utf8.encode("permission_flag")),
        defaultCacheState.publicKey.toBuffer(),
        reportHash,
      ],
      program.programId 
    );
    return pda
  }

  const decimalReportPDA = (dataId: Buffer) => {
    const [pda, _bump] = PublicKey.findProgramAddressSync(
      [
        Buffer.from(anchor.utils.bytes.utf8.encode("decimal_report")),
        defaultCacheState.publicKey.toBuffer(),
        dataId,
      ],
      program.programId 
    );
    return pda
  }

  const legacyWriterPDA = () => {
    const [pda, bump] = PublicKey.findProgramAddressSync(
      [
        Buffer.from(anchor.utils.bytes.utf8.encode("legacy_writer")),
        defaultCacheState.publicKey.toBuffer(),
      ],
      program.programId 
    );
    return pda
  }

  const legacyFeedsConfigPDA = () => {
    const [pda, _bump] = PublicKey.findProgramAddressSync(
      [
        Buffer.from(anchor.utils.bytes.utf8.encode("legacy_feeds_config")),
        defaultCacheState.publicKey.toBuffer()
      ],
      program.programId 
    );
    return pda
  }

  it("Initialize Cache", async () => {

    await program.methods
      .initialize([feedAdminA.provider.publicKey]) // todo: add owner here as well
      .accounts({
        state: defaultCacheState.publicKey,
        owner: provider.publicKey,
        systemProgram: anchor.web3.SystemProgram.programId
      })
      .signers([defaultCacheState])
      .rpc();
    
    const actualCacheState = await program.account.cacheState.fetch(defaultCacheState.publicKey);
    
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
      xs: [feedAdminA.provider.publicKey]
    };

    assert.isTrue(
      arrayVecEquals(expectedArrayVec, actualCacheState.feedAdmins, (a, b) => a.equals(b)),
      "feed admins equal" 
    )

    const [_pda, bump] = PublicKey.findProgramAddressSync(
      [
        Buffer.from(anchor.utils.bytes.utf8.encode("legacy_writer")),
        defaultCacheState.publicKey.toBuffer(),
      ],
      program.programId 
    );

    assert.equal(actualCacheState.legacyWriterNonce, bump, "legacy writer nonces equal");

  });

  describe("Legacy Feed Config Operations", function () {
    const legacyFeedConfigAccount = legacyFeedsConfigPDA();

    console.log("LEGACY CONFIG ACC", legacyFeedConfigAccount.toBase58());

    it("Initialize", async () => {

      await program
        .methods
        .initLegacyFeedsConfig([feedD.dataId] as any)
        .accounts({
          owner: provider.publicKey,
          state: defaultCacheState.publicKey,
          legacyStore: mockLegacyStore.programId, 
          legacyFeedsConfig: legacyFeedConfigAccount,
          systemProgram: anchor.web3.SystemProgram.programId,
        })
        .remainingAccounts([{
          pubkey: feedAdminA.keypair.publicKey,     // again, just use a dummy value
          isSigner: false,
          isWritable: true
        }])
        .rpc();

        const actualState = await program.account.legacyFeedsConfig.fetch(legacyFeedConfigAccount);

        const expectedEntry: ArrayVec<LegacyFeedEntry> = {
          len: new BN(1),
          xs: [{
            dataId: Array.from(feedD.dataId),
            legacyFeed: feedAdminA.keypair.publicKey,
            writeDisabled: 0
          }]
        };
        
        const entryEq = (a: LegacyFeedEntry, b: LegacyFeedEntry) => {
            return a.legacyFeed.equals(b.legacyFeed) && Buffer.from(a.dataId).equals(Buffer.from(b.dataId));
        }
  
        assert.isTrue(
          arrayVecEquals(expectedEntry, actualState.idToFeed, entryEq),
          "workflow metadata equal"
        )

        assert.isTrue(actualState.legacyStore.equals(mockLegacyStore.programId));

    });

    it("Update", async () => {

      const dataIds = [feedD.dataId, feedC.dataId].sort((a, b) => a.compare(b))

      await program
      .methods
      .updateLegacyFeedsConfig(
        dataIds as any, 
        [true, true]
      )
      .accounts({
        owner: provider.publicKey,
        state: defaultCacheState.publicKey,
        legacyStore: mockLegacyStore.programId, // todo: make sure you verify the feeds passed in
        legacyFeedsConfig: legacyFeedConfigAccount,
      })
      .remainingAccounts([
        {
          pubkey: reportSender.keypair.publicKey,     // again, just use a dummy value
          isSigner: false,
          isWritable: true
        },
        {
          pubkey: feedAdminA.keypair.publicKey,     // again, just use a dummy value
          isSigner: false,
          isWritable: true
        },

      ])
      .rpc();

      const actualState = await program.account.legacyFeedsConfig.fetch(legacyFeedConfigAccount);

      console.log('legfeedaccou', actualState);

      console.log(actualState.idToFeed.xs[0], 'xtra small')

      const expectedEntry: ArrayVec<LegacyFeedEntry> = {
        len: new BN(2),
        xs: [
          {
            dataId: Array.from(dataIds[0]),
            legacyFeed: reportSender.keypair.publicKey,
            writeDisabled: 1,
          },
          {
            dataId: Array.from(dataIds[1]),
            legacyFeed: feedAdminA.keypair.publicKey,
            writeDisabled: 1,
          },
        ]
      };
      
      const entryEq = (a: LegacyFeedEntry, b: LegacyFeedEntry) => {
          return a.legacyFeed.equals(b.legacyFeed) && Buffer.from(a.dataId).equals(Buffer.from(b.dataId)) && a.writeDisabled == b.writeDisabled;
      }

      assert.isTrue(
        arrayVecEquals(expectedEntry, actualState.idToFeed, entryEq),
        "entries equal"
      )

      assert.isTrue(actualState.legacyStore.equals(mockLegacyStore.programId));

    });

    // todo: unclose
    // it("Close", async () => {
    //   await program
    //   .methods
    //   .closeLegacyFeedsConfig()
    //   .accounts({
    //     owner: provider.publicKey,
    //     state: defaultCacheState.publicKey,
    //     legacyFeedsConfig: legacyFeedConfigAccount
    //   })
    //   .rpc();

    //   assert.isRejected(program.account.writePermissionFlag.fetch(legacyFeedConfigAccount), /Account does not exist/);




    // });

  })
  

  // todo: add more tests -- 
  // b. if you pass in the wrong length
  // c. if feed admin is not authorized
  // d. out of order data ids and out of order remaining accounts
  it("Initialize data feed reports", async () => {

    const feedAReportPDA = decimalReportPDA(feedA.dataId);
    const feedBReportPDA = decimalReportPDA(feedB.dataId);
    const feedCReportPDA = decimalReportPDA(feedC.dataId);

    // test initialization
    await program.methods
      .initDecimalReports([feedA.dataId] as any)
      .accounts({
        feedAdmin: feedAdminA.provider.publicKey,
        state: defaultCacheState.publicKey,
        systemProgram: anchor.web3.SystemProgram.programId
      })
      .remainingAccounts([
        {
          pubkey: feedAReportPDA,
          isSigner: false,
          isWritable: true
        }
      ])
      .signers([feedAdminA.keypair])
      .rpc();

      let feedAState = await program.account.decimalReport.fetch(feedAReportPDA);
      assert.equal(feedAState.timestamp, 0, "timestamp 0");

      // test initialization with existing feed as well
      await program.methods
      .initDecimalReports([feedA.dataId, feedB.dataId, feedC.dataId] as any)
      .accounts({
        feedAdmin: feedAdminA.provider.publicKey,
        state: defaultCacheState.publicKey,
        systemProgram: anchor.web3.SystemProgram.programId
      })
      .remainingAccounts([
        {
          pubkey: feedAReportPDA,
          isSigner: false,
          isWritable: true
        },
        {
          pubkey: feedBReportPDA,
          isSigner: false,
          isWritable: true
        },
        {
          pubkey: feedCReportPDA,
          isSigner: false,
          isWritable: true
        }
      ])
      .signers([feedAdminA.keypair])
      .rpc();

      feedAState = await program.account.decimalReport.fetch(feedAReportPDA);
      assert.equal(feedAState.timestamp, 0, "timestamp 0");

      const feedBState = await program.account.decimalReport.fetch(feedBReportPDA);
      assert.equal(feedBState.timestamp, 0, "timestamp 0");

      const feedCState = await program.account.decimalReport.fetch(feedCReportPDA);
      assert.equal(feedCState.timestamp, 0, "timestamp 0");
  });

  it("Set Feed Configs + Close Stale Accounts", async () => {

    const dataIds = [feedA.dataId];

    const descriptions = [feedA.description];

    const workflowMetadatas = [
      randomWorkflowMetadata(reportSender.provider.publicKey)
    ];

    const reportHash = getReportHash(
      dataIds[0], 
      workflowMetadatas[0].allowedSender.toBuffer(),
      workflowMetadatas[0].allowedWorkflowOwner,
      workflowMetadatas[0].allowedWorkflowName
    );

    // find the PDAs

    const feedConfigAccount1 = feedConfigPDA(dataIds[0]);
    const permissionFlagAccount1 = permissionFlagPDA(reportHash);

    await program.methods
      .setDecimalFeedConfigs(
        dataIds as any,
        descriptions as any,
        workflowMetadatas
      )
      .accounts({
        feedAdmin: feedAdminA.provider.publicKey, // todo: do we need to use owner here isntead? (probably not)
        state: defaultCacheState.publicKey,
        systemProgram: anchor.web3.SystemProgram.programId
      })
      .remainingAccounts([
        {
          pubkey: feedConfigAccount1,
          isSigner: false,
          isWritable: true
        },
        {
          pubkey: permissionFlagAccount1,
          isSigner: false,
          isWritable: true
        }
      ])
      .signers([feedAdminA.keypair]) // todo: 
      .rpc();

      console.log('past 1st call')
    
      const actualWritePermissionFlag = await program.account.writePermissionFlag.fetch(permissionFlagAccount1);

      assert.isTrue(Object.keys(actualWritePermissionFlag).length === 0, 'flag exists');

      const actualFeedConfig = await program.account.feedConfig.fetch(feedConfigAccount1);

      // actualFeedConfig.description 

      // console.log(actualFeedConfig, 'dat feed config doe');

      // console.log(actualFeedConfig.workflowMetadata.xs[0], 'dat entry doe');

      assert.isTrue(Buffer.from(actualFeedConfig.description).equals(descriptions[0]), 'descriptions equal');

      const expectedWorkflowMetadas: ArrayVec<WorkflowMetadata> = {
        len: new BN(1),
        xs: workflowMetadatas.map(x => ({
          allowedSender: x.allowedSender,
          allowedWorkflowOwner: Array.from(x.allowedWorkflowOwner),
          allowedWorkflowName: Array.from(x.allowedWorkflowName)
        }))
      };

      const workflowMetadataEq = (a: WorkflowMetadata, b: WorkflowMetadata) => {
          return a.allowedSender.equals(b.allowedSender) && 
          JSON.stringify(a.allowedWorkflowName) === JSON.stringify(b.allowedWorkflowName) &&
          JSON.stringify(a.allowedWorkflowOwner) === JSON.stringify(b.allowedWorkflowOwner);
      }

      assert.isTrue(
        arrayVecEquals(expectedWorkflowMetadas, actualFeedConfig.workflowMetadata, workflowMetadataEq),
        "workflow metadata equal"
      )

      assert.isTrue(
        arrayVecEquals<PublicKey>(
          { xs: [], len: new BN(0) }, 
          actualFeedConfig.stalePermissionAccounts, 
          (a, b) => (a.equals(b))),
        "stale accounts empty"
      )

    // todo: should work with 0 accounts?
      // todo: one update and one new

      const dataIds2 = [feedA.dataId, feedB.dataId];

      const descriptions2 = [randomDescription(), feedB.description]; // change assetA's description while we're at it!

      const workflowMetadatas2 = Array.from({length: 3}).map(() => {
        return randomWorkflowMetadata(reportSender.provider.publicKey)
      });

      const feedConfigAccounts2 = dataIds2.map(id => (feedConfigPDA(id)));
      // const permissionFlagAccounts2 = dataIds2.map(id => (feedConfigPDA(id)))

      const reportHashes2: Buffer[] = dataIds2.flatMap(dataId =>
        workflowMetadatas2.map(metadata =>
          getReportHash(
            dataId,
            metadata.allowedSender.toBuffer(),
            metadata.allowedWorkflowOwner,
            metadata.allowedWorkflowName
          )
        )
      );
      
      const permissionFlagAccounts2 = reportHashes2.map(hash => (permissionFlagPDA(hash)));

      const remainingAccounts = feedConfigAccounts2.map(acc => ({pubkey: acc, isSigner: false, isWritable: true})).concat(
        permissionFlagAccounts2.map(acc => ({pubkey: acc, isSigner: false, isWritable: true}))
      )


    await program.methods
    .setDecimalFeedConfigs(
      dataIds2 as any,
      descriptions2 as any,
      workflowMetadatas2
    )
    .accounts({
      feedAdmin: feedAdminA.provider.publicKey, // todo: do we need to use owner here isntead? (probably not)
      state: defaultCacheState.publicKey,
      systemProgram: anchor.web3.SystemProgram.programId
    })
    .remainingAccounts(remainingAccounts)
    .signers([feedAdminA.keypair]) // todo: 
    .rpc();

    // check the state of stuff here

    const expectedWorkflowMetadas2: ArrayVec<WorkflowMetadata> = {
      len: new BN(3),
      xs: workflowMetadatas2.map(x => ({
        allowedSender: x.allowedSender,
        allowedWorkflowOwner: Array.from(x.allowedWorkflowOwner),
        allowedWorkflowName: Array.from(x.allowedWorkflowName)
      }))
    };

    let expectedStaleAccounts = [
      { xs: [permissionFlagAccount1], len: new BN(1) },
      { xs: [], len: new BN(0) }
    ];

    for (let i = 0 ; i < dataIds2.length; i++) {
      const actualFeedConfigAsset = await program.account.feedConfig.fetch(feedConfigAccounts2[i]);
      assert.isTrue(Buffer.from(actualFeedConfigAsset.description).equals(descriptions2[i]), 'descriptions equal');
      assert.isTrue(
        arrayVecEquals(expectedWorkflowMetadas2, actualFeedConfigAsset.workflowMetadata, workflowMetadataEq),
        "workflow metadata equal"
      )
      assert.isTrue(
        arrayVecEquals<PublicKey>(
          expectedStaleAccounts[i], 
          actualFeedConfigAsset.stalePermissionAccounts, 
          (a, b) => (a.equals(b))),
        "stale accounts equal"
      )
    }

    const remainingAccountsClose = feedConfigAccounts2.map(acc => ({pubkey: acc, isSigner: false, isWritable: true})).concat(
      [permissionFlagAccount1].map(acc => ({pubkey: acc, isSigner: false, isWritable: true}))
    )

    console.log('we made it to the close');
    // todo: add data accounts which don't have any stale accounts
    // close stale accounts
    await program.methods
      .closeStalePermissionAccounts(dataIds2 as any)
      .accounts({
        feedAdmin: feedAdminA.provider.publicKey,
        state: defaultCacheState.publicKey,
      })
      .remainingAccounts(remainingAccountsClose)
      .signers([feedAdminA.keypair])
      .rpc();

    expectedStaleAccounts = [
      { xs: [], len: new BN(0) },
      { xs: [], len: new BN(0) }
    ];

    for (let i = 0 ; i < dataIds2.length; i++) {
      const actualFeedConfigAsset = await program.account.feedConfig.fetch(feedConfigAccounts2[i]);
      assert.isTrue(Buffer.from(actualFeedConfigAsset.description).equals(descriptions2[i]), 'descriptions equal');
      assert.isTrue(
        arrayVecEquals(expectedWorkflowMetadas2, actualFeedConfigAsset.workflowMetadata, workflowMetadataEq),
        "workflow metadata equal"
      )
      assert.isTrue(
        arrayVecEquals<PublicKey>(
          expectedStaleAccounts[i], 
          actualFeedConfigAsset.stalePermissionAccounts, 
          (a, b) => (a.equals(b))),
        "stale accounts equal"
      )
    }

    assert.isRejected(program.account.writePermissionFlag.fetch(permissionFlagAccount1), /Account does not exist/);

  });

  describe("updates reports", function () {

    const forwarder = new Forwarder(forwarderProgram, provider)
    .withState(Keypair.generate())
    .withOracles(1, 12, 41);

    let legacyFeeds: Feed[];
    let legacyWorkflowMetadatas: {
      allowedSender: anchor.web3.PublicKey;
      allowedWorkflowOwner: Buffer;
      allowedWorkflowName: Buffer;
    }[];
    let legacyReportHashes: Buffer[];

    before(async () => {
      await forwarder.initialize();
      await forwarder.initOraclesConfig();

      legacyFeeds = [feedC, feedD].sort((a, b) => a.dataId.compare(b.dataId));
      legacyWorkflowMetadatas = newWorkflows(1, forwarder.forwarderAuthority[0]);
      legacyReportHashes = legacyFeeds.map((f) => {
        return getReportHash(
          f.dataId, 
          legacyWorkflowMetadatas[0].allowedSender.toBuffer(),
          legacyWorkflowMetadatas[0].allowedWorkflowOwner,
          legacyWorkflowMetadatas[0].allowedWorkflowName
        );
      });

      await program.methods
      .setDecimalFeedConfigs(
        legacyFeeds.map(f => f.dataId) as any,
        legacyFeeds.map(f => f.description) as any,
        legacyWorkflowMetadatas
      )
      .accounts({
        feedAdmin: feedAdminA.provider.publicKey, // todo: do we need to use owner here instead? (probably not)
        state: defaultCacheState.publicKey,
        systemProgram: anchor.web3.SystemProgram.programId
      })
      .remainingAccounts([
        {
          pubkey: feedConfigPDA(legacyFeeds[0].dataId),
          isSigner: false,
          isWritable: true
        },
        {
          pubkey: feedConfigPDA(legacyFeeds[1].dataId),
          isSigner: false,
          isWritable: true
        },
        {
          pubkey: permissionFlagPDA(legacyReportHashes[0]),
          isSigner: false,
          isWritable: true
        },
        {
          pubkey: permissionFlagPDA(legacyReportHashes[1]),
          isSigner: false,
          isWritable: true
        }
      ])
      .signers([feedAdminA.keypair])
      .rpc();

      await program.methods
      .initDecimalReports([feedC.dataId, feedD.dataId] as any)
      .accounts({
        feedAdmin: feedAdminA.provider.publicKey,
        state: defaultCacheState.publicKey,
        systemProgram: anchor.web3.SystemProgram.programId
      })
      .remainingAccounts([
        {
          pubkey: decimalReportPDA(feedC.dataId),
          isSigner: false,
          isWritable: true
        },
        {
          pubkey: decimalReportPDA(feedD.dataId),
          isSigner: false,
          isWritable: true
        }
      ])
      .signers([feedAdminA.keypair])
      .rpc();

    })

    it("write to legacy feeds", async () => {

      const feedReports = legacyFeeds.map((f, i) => {
        return program.coder.types.encode("ReceivedDecimalReport", {
          timestamp: new BN(i+10),
          answer: new BN(i+10),
          dataId: f.dataId
        })
      });

      const lenPrefix = Buffer.alloc(4);
      lenPrefix.writeUInt32LE(2, 0);

      const fullEncodedVec = Buffer.concat([lenPrefix].concat(feedReports));


      // console.log('mockLegacyStore.programId', mockLegacyStore.programId);

      const dummyLegacyFeedAccs = [
        {
          pubkey: reportSender.keypair.publicKey,     // again, just use a dummy value
          isSigner: false,
          isWritable: true
        },
        {
          pubkey: feedAdminA.keypair.publicKey,     // again, just use a dummy value
          isSigner: false,
          isWritable: true
        },

      ].sort((a,b) => a.pubkey.toBuffer().compare(b.pubkey.toBuffer()))

      console.log(`before report!!!!! workflow name: ${new Uint8Array(legacyWorkflowMetadatas[0].allowedWorkflowName)}
        workflow owner: ${new Uint8Array(legacyWorkflowMetadatas[0].allowedWorkflowOwner)}
        `)

        console.log(`report hashes:  1st is ${new Uint8Array(legacyReportHashes[0])} 2nd is ${new Uint8Array(legacyReportHashes[1])}`)

// todo: need to init decimal reports


       // make it work with the right report hash permissions
       await forwarder.report(
        program.programId,
        fullEncodedVec,
        legacyWorkflowMetadatas[0].allowedWorkflowName,
        legacyWorkflowMetadatas[0].allowedWorkflowOwner,
       [ {
          pubkey: defaultCacheState.publicKey,
          isSigner: false,
          isWritable: false,
          },
          {
            pubkey: mockLegacyStore.programId, // legacy store
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: legacyFeedsConfigPDA(), // legacy feed config
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: legacyWriterPDA(), // legacy writer
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: anchor.web3.SystemProgram.programId,
            isSigner: false,
            isWritable: false,
          },
          {
            pubkey: decimalReportPDA(legacyFeeds[0].dataId),
            isSigner: false,
            isWritable: true,
          },
          {
            pubkey: decimalReportPDA(legacyFeeds[1].dataId),
            isSigner: false,
            isWritable: true,
          },
          {
            pubkey: permissionFlagPDA(legacyReportHashes[0]),
            isSigner: false,
            isWritable: false
          },
          {
            pubkey: permissionFlagPDA(legacyReportHashes[1]),
            isSigner: false,
            isWritable: false
          },
          dummyLegacyFeedAccs[0],
          dummyLegacyFeedAccs[1],
       ]
        
      )

      // todo: assert for all the report states

           const report1 = await program.account.decimalReport.fetch(decimalReportPDA(legacyFeeds[0].dataId));
      assert.isTrue(report1.answer.eq(new BN(10)), 'answers match');
      assert.isTrue(report1.timestamp == 10, 'answers match');

         const report2 = await program.account.decimalReport.fetch(decimalReportPDA(legacyFeeds[1].dataId));
      assert.isTrue(report2.answer.eq(new BN(11)), 'answers match');
      assert.isTrue(report2.timestamp == 11, 'answers match');
  


      


    });

    it("without legacy feeds", async () => {
  
      // first we must update the workflow metadata to work with the desired receiver
  
      const workflowMetadatasC = newWorkflows(1, forwarder.forwarderAuthority[0])
  
      const reportHash = getReportHash(
        feedC.dataId, 
        legacyWorkflowMetadatas[0].allowedSender.toBuffer(),
        legacyWorkflowMetadatas[0].allowedWorkflowOwner,
        legacyWorkflowMetadatas[0].allowedWorkflowName
      );
  
      const singleReport = program.coder.types.encode("ReceivedDecimalReport", {
        timestamp: new BN(123),
        answer: new BN(321),
        dataId: feedC.dataId
      });

      const lenPrefix = Buffer.alloc(4);
      lenPrefix.writeUInt32LE(1, 0);
  
      // Step 3: Concatenate length + all reports
      const fullEncodedVec = Buffer.concat([lenPrefix, singleReport]);
  
      const feedCReportPDA = decimalReportPDA(feedC.dataId);
      const permissionFlagCPDA = permissionFlagPDA(reportHash);
  
  
      console.log('data id', feedC.dataId, `forwarderAUthority ${forwarder.forwarderAuthority[0].toBase58()}`,
      'workflow owner:', workflowMetadatasC[0].allowedWorkflowOwner, 'workflow name:', workflowMetadatasC[0].allowedWorkflowName)
  
  
      // make it work with the right report hash permissions
        await forwarder.report(
          program.programId,
          fullEncodedVec,
          legacyWorkflowMetadatas[0].allowedWorkflowName,
          legacyWorkflowMetadatas[0].allowedWorkflowOwner,
         [ {
            pubkey: defaultCacheState.publicKey,
            isSigner: false,
            isWritable: false,
            },
            {
              pubkey: program.programId, // legacy store (omitted)
              isSigner: false,
              isWritable: false,
            },
            {
              pubkey: program.programId, // legacy feed config (omitted)
              isSigner: false,
              isWritable: false,
            },
            {
              pubkey: program.programId, // legacy writer (omitted)
              isSigner: false,
              isWritable: false,
            },
            {
              pubkey: anchor.web3.SystemProgram.programId,
              isSigner: false,
              isWritable: false,
            },
            {
              pubkey: feedCReportPDA,
              isSigner: false,
              isWritable: true
            },
            {
              pubkey: permissionFlagCPDA,
              isSigner: false,
              isWritable: true
            },
         ]
          
        )
  
  

  
    })

  });


 

  

  // todo: clear the stale accounts
  // access the feedConfig accounts
  // access the stale writePermission accounts

});