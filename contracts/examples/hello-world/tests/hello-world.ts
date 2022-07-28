import * as anchor from "@project-serum/anchor";
import * as fs from "fs";
import { Program, BN } from "@project-serum/anchor";
import { HelloWorld } from "../target/types/hello_world";

const CHAINLINK_PROGRAM_ID = "HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny";

describe("hello-world", () => {
  const provider = anchor.AnchorProvider.env();

  // Configure the client to use the local cluster.
  anchor.setProvider(provider);

  const program = anchor.workspace.HelloWorld as Program<HelloWorld>;

  it("Is initialized!", async () => {
    const owner = provider.wallet;
    const store = anchor.web3.Keypair.generate();
    const feed = anchor.web3.Keypair.generate();
    const accessController = anchor.web3.Keypair.generate();

    let storeIdl = JSON.parse(fs.readFileSync("../../target/idl/store.json"));
    const storeProgram = new Program(storeIdl, CHAINLINK_PROGRAM_ID, provider);

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
          feed: feed.publicKey,
          authority: owner.publicKey,
        },
        signers: [feed],
        preInstructions: [
          await storeProgram.account.transmissions.createInstruction(
            feed,
            8 + 192 + 6 * 24
          ),
        ],
      }
    );

    await storeProgram.rpc.setWriter(owner.publicKey, {
      accounts: {
        feed: feed.publicKey,
        owner: owner.publicKey,
        authority: owner.publicKey,
      },
    });

    const scale = new BN(10).pow(new BN(decimals));
    // Scale answer to enough decimals
    let answer = new BN(1).mul(scale);
    let round = { timestamp: new BN(1), answer };

    let tx = await storeProgram.rpc.submit(round, {
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
        chainlinkProgram: CHAINLINK_PROGRAM_ID,
      },
      options: { commitment: "confirmed" },
    });
    console.log("Your transaction signature", tx);
    let t = await provider.connection.getConfirmedTransaction(tx, "confirmed");
    console.log(t.meta.logMessages);
  });
});
