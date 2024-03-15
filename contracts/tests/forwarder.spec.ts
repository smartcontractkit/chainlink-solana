import * as anchor from "@coral-xyz/anchor";
import { ProgramError, BN } from "@coral-xyz/anchor";
import { Keypair, LAMPORTS_PER_SOL, PublicKey } from "@solana/web3.js";
import * as borsh from "borsh";

import { randomBytes, createHash } from "crypto";
import * as secp256k1 from "secp256k1";
import { keccak256 } from "ethereum-cryptography/keccak";

import { assert } from "chai";
import { getOrCreateAssociatedTokenAccount } from "@solana/spl-token";

describe("ocr2", () => {
  // Configure the client to use the local cluster.
  const provider = anchor.AnchorProvider.local();
  anchor.setProvider(provider);

  const forwarderProgram = anchor.workspace.KeystoneForwarder;

  // Generate a new wallet keypair and airdrop SOL
  const payer = Keypair.generate();

  const owner = provider.wallet;

  const state = Keypair.generate();
  const transmitter = Keypair.generate();

  let authorityNonce;
  let authority: PublicKey;

  let oracles = [];
  const f = 6;
  // NOTE: 17 is the most we can fit into one proposeConfig if we use a different payer
  // if the owner == payer then we can fit 19
  const n = 19; // min: 3 * f + 1;

  let generateOracle = async () => {
    let secretKey = randomBytes(32);
    let transmitter = Keypair.generate();
    return {
      signer: {
        secretKey,
        publicKey: secp256k1.publicKeyCreate(secretKey, false).slice(1), // compressed = false, skip first byte (0x04)
      },
      transmitter,
    };
  };

  it("Funds the payer", async () => {
    await provider.connection.confirmTransaction(
      await provider.connection.requestAirdrop(payer.publicKey, LAMPORTS_PER_SOL * 1000),
      "confirmed"
    );

    await provider.connection.confirmTransaction(
      await provider.connection.requestAirdrop(transmitter.publicKey, LAMPORTS_PER_SOL * 1000),
      "confirmed"
    );
  });

  it("Initializes the forwarder", async() => {
    await forwarderProgram.methods
      .initialize()
      .accounts({
        state: state.publicKey,
        owner: owner.publicKey,
      })
      .signers([state])
      // .preInstructions([await program.account.state.createInstruction(state)])
      .rpc();

    let stateAccount = await forwarderProgram.account.state.fetch(state.publicKey);
    authorityNonce = stateAccount.authorityNonce;
    authority = PublicKey.createProgramAddressSync(
      [
        Buffer.from(anchor.utils.bytes.utf8.encode("forwarder")),
        state.publicKey.toBuffer(),
        Buffer.from([authorityNonce])
      ],
      forwarderProgram.programId
    );

    console.log(`Generating ${n} oracles...`);
    let futures = [];
    for (let i = 0; i < n; i++) {
      futures.push(generateOracle());
    }
    oracles = await Promise.all(futures);
  });

  // TODO: deploy mock receiver, forward the report there, assert on program log

  it("Successfully receives a new, valid report", async () => {
    const rawReport = Buffer.from([
      // 32 byte workflow id
      // 32 byte workflow execution id
      // report data
    ]);

    let hash = createHash("sha256")
      .update(rawReport)
      .digest();

    let rawSignatures = [];
    // for (let oracle of oracles.slice(0, f + 1)) {
    //   // sign with `f` + 1 oracles
    //   let { signature, recid } = secp256k1.ecdsaSign(
    //     hash,
    //     oracle.signer.secretKey
    //   );
    //   rawSignatures.push(...signature);
    //   rawSignatures.push(recid);
    // }

    let data = Buffer.concat([
      Buffer.from([rawSignatures.length]),
      Buffer.from(rawSignatures),
      rawReport,
    ]);

    const executionState = Keypair.generate();

    await forwarderProgram.methods
      .report(data)
      .accounts({
        state: state.publicKey,
        authority: transmitter.publicKey,
        forwarderAuthority: authority,
        executionState: executionState.publicKey, // TODO: derive
        receiverProgram: forwarderProgram.programId, // TODO:
      })
      .signers([transmitter])
      .rpc();

    // TODO: await until confirmation on all of these
    
  });

  it("Doesn't retransmit the same report", async () => {
    
  });
})
