import * as anchor from '@project-serum/anchor';
import * as fs from 'fs';
import { Program, BN } from '@project-serum/anchor';
import { ChainlinkSolanaDemo } from '../target/types/chainlink_solana_demo';
const assert = require("assert");

//const CHAINLINK_PROGRAM_ID = "A7Jh2nb1hZHwqEofm4N8SXbKTj82rx7KUfjParQXUyMQ";
const CHAINLINK_PROGRAM_ID = "DWqYEinRbZWtuq1DiDYvmexAKFoyjSyazZZUvdgPHT5g";
// SOL/USD
const CHAINLINK_FEED = "7ndYj66ec3yPS58kRpodch3n8TEkCiaiy8tZ8Szb3BjP";

describe('chainlink-solana-demo', () => {
  const provider = anchor.Provider.env();

  // Configure the client to use the local cluster.
  anchor.setProvider(provider);

  //const program = anchor.workspace.ChainlinkSolanaDemo as Program<ChainlinkSolanaDemo>;

  it('Query SOL/USD Price Feed!', async () => {

    const idl = JSON.parse(
      require("fs").readFileSync("./target/idl/chainlink_solana_demo.json", "utf8")
    );

    // Address of the deployed program.
    const programId = new anchor.web3.PublicKey("EsYPTcY4Be6GvxojV5kwZ7W2tK2hoVkm9XSN7Lk8HAs8");

    // Generate the program client from IDL.
    const program = new anchor.Program(idl, programId);

    //create an account to store the price data
    const decimal = anchor.web3.Keypair.generate();

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



    assert.ok(decimalAccount.value > 0);
  });
});
