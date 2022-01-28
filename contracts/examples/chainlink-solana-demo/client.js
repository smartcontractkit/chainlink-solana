const anchor = require("@project-serum/anchor");

// devnet program ID
const CHAINLINK_PROGRAM_ID = "DWqYEinRbZWtuq1DiDYvmexAKFoyjSyazZZUvdgPHT5g";
// SOL/USD feed on devnet
const CHAINLINK_FEED = "7ndYj66ec3yPS58kRpodch3n8TEkCiaiy8tZ8Szb3BjP";

const provider = anchor.Provider.env();

// Configure the cluster.
anchor.setProvider(provider);

async function main() {
  // Read the generated IDL.
  const idl = JSON.parse(
    require("fs").readFileSync("./target/idl/chainlink_solana_demo.json", "utf8")
  );

  // Address of the deployed program.
  const programId = new anchor.web3.PublicKey("JC16qi56dgcLoaTVe4BvnCoDL6FhH5NtahA7jmWZFdqm");

  // Generate the program client from IDL.
  const program = new anchor.Program(idl, programId);

  //create an account to store the price data
  const priceFeedAccount = anchor.web3.Keypair.generate();

  console.log('priceFeedAccount public key: ' + priceFeedAccount.publicKey);
  console.log('user public key: ' + provider.wallet.publicKey);

  // Execute the RPC.
  let tx = await program.rpc.execute({
    accounts: {
      decimal: priceFeedAccount.publicKey,
      user: provider.wallet.publicKey,
      chainlinkFeed: CHAINLINK_FEED,
      chainlinkProgram: CHAINLINK_PROGRAM_ID,
      systemProgram: anchor.web3.SystemProgram.programId
    },
    options: { commitment: "confirmed" },
    signers: [priceFeedAccount],
  });

  console.log("Fetching transaction logs...");
  let t = await provider.connection.getConfirmedTransaction(tx, "confirmed");
  console.log(t.meta.logMessages);
  // #endregion main

  // Fetch the account details of the account containing the price data
  const latestPrice = await program.account.decimal.fetch(priceFeedAccount.publicKey);
  console.log('Price Is: ' + latestPrice.value)
}

console.log("Running client...");
main().then(() => console.log("Success"));