import * as anchor from "@coral-xyz/anchor";
import { AnchorProvider, Program, Wallet, getProvider } from "@coral-xyz/anchor";
import { DataFeedsCache } from "../target/types/data_feeds_cache";
import { Keypair, LAMPORTS_PER_SOL, PublicKey } from "@solana/web3.js";
import { assert } from "chai";
// const BN = require('bn.js');
import BN from "bn.js";
import { randomBytes } from "crypto";

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

type ArrayVec<T> = {
  len: BN, // a bignumber,
  xs: Array<T>
}

type EqualsFn<T> = (a: T, b: T) => boolean;

// If expected array may be of smaller length than actual array
// We don't care about the rest of the entries since this is an arrayvec!() on-chain
function arrayVecEquals<T>(expected: ArrayVec<T>, actual: ArrayVec<T>, equalsFn: EqualsFn<T> ) {
  return expected.len.eq(actual.len) && expected.xs.reduce((equalsAcc, curr, index) => {
    return equalsAcc && equalsFn(curr, actual.xs[index]);
  }, true)
}

describe("data feeds cache", function () {
  // Configure the client to use the local cluster.
  const provider = anchor.AnchorProvider.env();
  anchor.setProvider(provider);


  const defaultConnection = getProvider().connection;

  let feedAdminA: Signer;
  
  const defaultCacheState = Keypair.generate();

  const program = anchor.workspace.DataFeedsCache as Program<DataFeedsCache>;

  before(async () => {
    feedAdminA = await newSigner(defaultConnection);
  })

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

  it("Set Feed Configs", async () => {



    const dataIds = [randomBytes(16)];

    const input = Buffer.from("to the moon!", "utf8");
    const paddedInput = Buffer.alloc(32);      // create 32-byte buffer filled with zeros
    input.copy(paddedInput);     

    const descriptions = [paddedInput];


    
    const workflowMetadatas = [
      {
        allowedSender: feedAdminA.provider.publicKey, // todo: replace with something else
        allowedWorkflowOwner: randomBytes(20),
        allowedWorkflowName: randomBytes(32),
      }
    ];

    console.log('the inputs: ', dataIds, descriptions, workflowMetadatas)

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
      .signers([feedAdminA.keypair]) // todo: 
      .rpc();
    

    // todo: should work with 0 accounts?



  });


});