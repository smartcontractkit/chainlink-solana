const anchor = require("@coral-xyz/anchor");

// devnet IDs
const CHAINLINK_PROGRAM_ID = "HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny";
// USDT/USD
const CHAINLINK_FEED = "CwBg8pxL73LvuJ781cWBGF1e64G2z7AbZ22J2g8Lp35a";

const provider = anchor.AnchorProvider.env();

// Configure the cluster.
anchor.setProvider(provider);

async function main() {
  // #region main
  // Read the generated IDL.
  const idl = JSON.parse(
    require("fs").readFileSync("./target/idl/hello_world.json", "utf8")
  );

  // Address of the deployed program.
  const programId = new anchor.web3.PublicKey("<YOUR-PROGRAM-ID>");

  // Generate the program client from IDL.
  const program = new anchor.Program(idl, programId);

  // Execute the RPC.
  let tx = await program.rpc.execute({
    accounts: {
      chainlinkFeed: CHAINLINK_FEED,
      chainlinkProgram: CHAINLINK_PROGRAM_ID,
    },
    options: { commitment: "confirmed" },
  });

  console.log("Fetching transaction logs...");
  let t = await provider.connection.getConfirmedTransaction(tx, "confirmed");
  console.log(t.meta.logMessages);
  // #endregion main
}

console.log("Running client...");
main().then(() => console.log("Success"));
