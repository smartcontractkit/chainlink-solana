import * as anchor from "@coral-xyz/anchor";
import { AnchorProvider, Program, Wallet, getProvider } from "@coral-xyz/anchor";
import { DataFeedsCache } from "../target/types/data_feeds_cache";
import { Keypair, LAMPORTS_PER_SOL, PublicKey } from "@solana/web3.js";
import BN from "bn.js";
import { createHash, randomBytes } from "crypto";
import * as chai from 'chai';
import chaiAsPromised from 'chai-as-promised';
import { assert } from "chai";

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

const randomFeedData = () => {
  return {
    dataId: randomBytes(16), 
    description: randomDescription()
  }
}; 

const randomWorkflowMetadata = (allowedSender: PublicKey) => {
  return {
    allowedSender: allowedSender, // todo: replace with something else
    allowedWorkflowOwner: randomBytes(20),
    allowedWorkflowName: randomBytes(32),
  }
}

describe("data feeds cache", function () {
  // Configure the client to use the local cluster.
  const provider = anchor.AnchorProvider.env();
  anchor.setProvider(provider);


  const defaultConnection = getProvider().connection;

  let feedAdminA: Signer;
  let reportSender: Signer;
  let otherSigners: Array<Signer>;
  
  const defaultCacheState = Keypair.generate();

  const program = anchor.workspace.DataFeedsCache as Program<DataFeedsCache>;

  before(async () => {
    [feedAdminA, reportSender, ...otherSigners] = await newSigners(defaultConnection, 5);
  });

  const feedConfigPDA = (dataId: Buffer) => {
    const [feedConfigAccount, _bump] = PublicKey.findProgramAddressSync(
      [
        Buffer.from(anchor.utils.bytes.utf8.encode("feed_config")),
        defaultCacheState.publicKey.toBuffer(),
        dataId,
      ],
      program.programId
    );
    return feedConfigAccount;
  }

  const permissionFlagPDA = (reportHash: Buffer) => {
    const [permissionFlagAccount, _bump] = PublicKey.findProgramAddressSync(
      [
        Buffer.from(anchor.utils.bytes.utf8.encode("permission_flag")),
        defaultCacheState.publicKey.toBuffer(),
        reportHash,
      ],
      program.programId 
    );
    return permissionFlagAccount
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

  });

  it("Set Feed Configs + Close Stale Accounts", async () => {

    const assetA = randomFeedData()

    const dataIds = [assetA.dataId];

    const descriptions = [assetA.description];

    const workflowMetadatas = [randomWorkflowMetadata(reportSender.provider.publicKey)];

    const reportHash = getReportHash(
      dataIds[0], 
      workflowMetadatas[0].allowedSender.toBuffer(),
      workflowMetadatas[0].allowedWorkflowOwner,
      workflowMetadatas[0].allowedWorkflowName
    );

    // find the PDAs

    const feedConfigAccount1 = feedConfigPDA(dataIds[0]);

    // const [feedConfigAccount, _feedConfigAccountBump] = PublicKey.findProgramAddressSync(
    //   [
    //     Buffer.from(anchor.utils.bytes.utf8.encode("feed_config")),
    //     dataIds[0],
    //   ],
    //   program.programId
    // );

    const permissionFlagAccount1 = permissionFlagPDA(reportHash);

    // const [permissionFlagAccount, _permissionFlagAccountBump] = PublicKey.findProgramAddressSync(
    //   [
    //     Buffer.from(anchor.utils.bytes.utf8.encode("permission_flag")),
    //     reportHash,
    //   ],
    //   program.programId 
    // );

    console.log('we reached the point')

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

      const assetB = randomFeedData()

      const dataIds2 = [assetA.dataId, assetB.dataId];

      const descriptions2 = [randomDescription(), assetB.description]; // change assetA's description while we're at it!

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

  

  // todo: clear the stale accounts
  // access the feedConfig accounts
  // access the stale writePermission accounts

});