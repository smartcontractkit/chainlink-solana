const anchor = require("@project-serum/anchor");

// devnet IDs
const CHAINLINK_PROGRAM_ID = "DWqYEinRbZWtuq1DiDYvmexAKFoyjSyazZZUvdgPHT5g";
// SOL/USD
const CHAINLINK_FEED = "7ndYj66ec3yPS58kRpodch3n8TEkCiaiy8tZ8Szb3BjP";

const provider = anchor.Provider.env();

// Configure the cluster.
anchor.setProvider(provider);

async function main() {
  // #region main
  // Read the generated IDL.
  const idl = JSON.parse(
    require("fs").readFileSync("./target/idl/chainlink_solana_demo.json", "utf8")
  );

  // Address of the deployed program.
  const programId = new anchor.web3.PublicKey("EsYPTcY4Be6GvxojV5kwZ7W2tK2hoVkm9XSN7Lk8HAs8");

  // Generate the program client from IDL.
  const program = new anchor.Program(idl, programId);

  //create an account to store the price data
  const decimal = anchor.web3.Keypair.generate();

  console.log('decimal public key: ' + decimal.publicKey);
  console.log('user public key: ' + provider.wallet.publicKey);
  console.log('system program ID:' + anchor.web3.SystemProgram.programId)

  // Execute the RPC.
  let tx = await program.rpc.execute({
    accounts: {
      decimal: decimal.publicKey,
      user: provider.wallet.publicKey,
      chainlinkFeed: CHAINLINK_FEED,
      chainlinkProgram: CHAINLINK_PROGRAM_ID,
      systemProgram: anchor.web3.SystemProgram.programId
    },
    options: { commitment: "confirmed" },
    signers: [decimal],
  });

  console.log("Fetching transaction logs...");
  let t = await provider.connection.getConfirmedTransaction(tx, "confirmed");
  console.log(t.meta.logMessages);
  // #endregion main

  // Fetch the account details of the account containing the price data
  const decimalAccount = await program.account.decimal.fetch(decimal.publicKey);
  console.log('Price Is: ' + decimalAccount.value)
}

console.log("Running client...");
main().then(() => console.log("Success"));