import * as anchor from "@coral-xyz/anchor";
import * as fs from "fs";
import { Program, BN } from "@coral-xyz/anchor";
import { HelloWorld } from "../target/types/hello_world";

// Must match `store` in `contracts/Anchor.toml` and `programs/store` `declare_id!`.
const CHAINLINK_PROGRAM_ID = "9kRNTZmoZSiTBuXC62dzK9E7gC7huYgcmRRhYv3i4osC";

describe("hello-world", () => {
  const provider = anchor.AnchorProvider.env();

  // Configure the client to use the local cluster.
  anchor.setProvider(provider);

  const program = anchor.workspace.HelloWorld as Program<HelloWorld>;

  const header = 8 + 192; // account discriminator + header
  const transmissionSize = 48;

  it("Is initialized!", async () => {
    const owner = provider.wallet;
    const store = anchor.web3.Keypair.generate();
    const feed = anchor.web3.Keypair.generate();

    let storeIdl = JSON.parse(
      fs.readFileSync("../../target/idl/store.json", "utf-8")
    );
    storeIdl.address = CHAINLINK_PROGRAM_ID;
    const storeProgram = new Program(storeIdl, provider);

    // Create a feed
    const description = "FOO/BAR";
    const decimals = 18;
    const granularity = 30;
    // chainlink_solana v2 read_feed_v2 requires exactly one live transmission
    const liveLength = 1;
    const historicalLength = 3;
    await storeProgram.methods
      .createFeed(description, decimals, granularity, liveLength)
      .accounts({
        feed: feed.publicKey,
        authority: owner.publicKey,
      })
      .signers([feed])
      .preInstructions([
        await storeProgram.account.transmissions.createInstruction(
          feed,
          header + (liveLength + historicalLength) * transmissionSize
        ),
      ])
      .rpc({ commitment: "confirmed" });
    console.log("deployed store");

    await storeProgram.methods
      .setWriter(owner.publicKey)
      .accounts({
        feed: feed.publicKey,
        owner: owner.publicKey,
        authority: owner.publicKey,
      })
      .rpc({ commitment: "confirmed" });
    console.log("set writer on store");

    const scale = new BN(10).pow(new BN(decimals));
    // Scale answer to enough decimals
    let answer = new BN(1).mul(scale);
    let round = { timestamp: new BN(1), answer };

    let tx = await storeProgram.methods
      .submit(round)
      .accounts({
        store: store.publicKey,
        feed: feed.publicKey,
        authority: owner.publicKey,
      })
      .rpc({ commitment: "confirmed" });
    console.log("value written to store");

    // Add your test here.
    tx = await program.methods
      .execute()
      .accounts({
        chainlinkFeed: feed.publicKey,
        chainlinkProgram: CHAINLINK_PROGRAM_ID,
      })
      .rpc({ commitment: "confirmed" });
    console.log("Your transaction signature", tx);
    let t = await provider.connection.getTransaction(tx, {
      commitment: "confirmed",
    });
    console.log(t.meta.logMessages);
  });
});
