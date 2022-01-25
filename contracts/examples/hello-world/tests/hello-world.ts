import * as anchor from '@project-serum/anchor';
import * as fs from 'fs';
import { Program, BN } from '@project-serum/anchor';
import { HelloWorld } from '../target/types/hello_world';

const CHAINLINK_PROGRAM_ID = "A7Jh2nb1hZHwqEofm4N8SXbKTj82rx7KUfjParQXUyMQ";

describe('hello-world', () => {
  const provider = anchor.Provider.env();

  // Configure the client to use the local cluster.
  anchor.setProvider(provider);

  const program = anchor.workspace.HelloWorld as Program<HelloWorld>;

  it('Is initialized!', async () => {
    const owner = provider.wallet;
    const store = anchor.web3.Keypair.generate();
    const feed = anchor.web3.Keypair.generate();
    const accessController = anchor.web3.Keypair.generate();

    let storeIdl = JSON.parse(fs.readFileSync('../../target/idl/store.json'));    
    const storeProgram = new Program(storeIdl, CHAINLINK_PROGRAM_ID, provider);

    let acIdl = JSON.parse(fs.readFileSync('../../target/idl/access_controller.json'));    
    const accessControllerProgram = new Program(acIdl, "2F5NEkMnCRkmahEAcQfTQcZv1xtGgrWFfjENtTwHLuKg", provider);

    await accessControllerProgram.rpc.initialize({
      accounts: {
        state: accessController.publicKey,
        payer: provider.wallet.publicKey,
        owner: owner.publicKey,
        rent: anchor.web3.SYSVAR_RENT_PUBKEY,
        systemProgram: anchor.web3.SystemProgram.programId,
      },
      signers: [accessController],
      preInstructions: [
        await accessControllerProgram.account.accessController.createInstruction(accessController),
      ],
    });

    // Initialize a new store
    await storeProgram.rpc.initialize({
      accounts: {
        store: store.publicKey,
        owner: owner.publicKey,
        loweringAccessController: accessController.publicKey,
      },
      signers: [store],
      preInstructions: [
        await storeProgram.account.store.createInstruction(store),
      ],
    });

    // Create a feed
    const description = "FOO/BAR";
    const decimals = 18;
    const granularity = 30;
    const liveLength = 3;
    await storeProgram.rpc.createFeed(
      description,
      decimals,
      granularity,
      liveLength,
    {
      accounts: {
        store: store.publicKey,
        feed: feed.publicKey,
        authority: owner.publicKey,
      },
      signers: [feed],
      preInstructions: [
        await storeProgram.account.transmissions.createInstruction(feed, 8+128+6*24),
      ],
    });

    await storeProgram.rpc.setWriter(
      owner.publicKey,
      {
        accounts: {
          store: store.publicKey,
          feed: feed.publicKey,
          authority: owner.publicKey,
        },
      });
    

    const scale = (new BN(10)).pow(new BN(decimals));
    // Scale answer to enough decimals
    let answer = (new BN(1)).mul(scale);
    let round = { timestamp: new BN(1), answer };

    let tx = await storeProgram.rpc.submit(
      round,
      {
        accounts: {
          store: store.publicKey,
          feed: feed.publicKey,
          authority: owner.publicKey,
        },
      });
    await provider.connection.confirmTransaction(tx);

    // Add your test here.
    tx = await program.rpc.execute({
        accounts: {
          chainlinkFeed: feed.publicKey,
          chainlinkProgram: CHAINLINK_PROGRAM_ID
        },
        options: { commitment: "confirmed" },
    });
    console.log("Your transaction signature", tx);
    let t = await provider.connection.getConfirmedTransaction(tx, "confirmed");
    console.log(t.meta.logMessages)
  });
});
