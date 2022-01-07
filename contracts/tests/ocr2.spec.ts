import * as anchor from "@project-serum/anchor";
import { ProgramError, BN } from "@project-serum/anchor";
import {
	SYSVAR_RENT_PUBKEY,
	PublicKey,
	Keypair,
	SystemProgram,
  Transaction,
  TransactionInstruction,
} from '@solana/web3.js';
import {
  Token,
  ASSOCIATED_TOKEN_PROGRAM_ID,
  TOKEN_PROGRAM_ID,
} from '@solana/spl-token';
import { assert } from "chai";

import { randomBytes, createHash } from "crypto";
import * as secp256k1 from "secp256k1";
import { keccak256 } from "ethereum-cryptography/keccak";

// generate a new keypair using `solana-keygen new -o id.json`

let ethereumAddress = (publicKey: Buffer) => {
  return keccak256(publicKey).slice(12)
};

const Scope = {
  Version: { version: {} },
  // RoundData: { roundData: { roundId } },
  LatestRoundData: { latestRoundData: {} },
  Aggregator: { aggregator: {} },
};

describe('ocr2', async () => {
  // Configure the client to use the local cluster.
  const provider = anchor.Provider.env();
  anchor.setProvider(provider);

  const billingAccessController = Keypair.generate();
  const requesterAccessController = Keypair.generate();
  const store = Keypair.generate();
  const flaggingThreshold = 80000;

  const observationPayment = 1;

  const state = Keypair.generate();
  // const stateSize = 8 + ;
  const transmissions = Keypair.generate();
  const payer = Keypair.generate();
  // const owner = Keypair.generate();
  const owner = provider.wallet;
  const mintAuthority = Keypair.generate();

  const placeholder = Keypair.generate().publicKey;

  const decimals = 18;
  const description = "ETH/BTC";

  const minAnswer = 1;
  const maxAnswer = 1000;

  const workspace = anchor.workspace;
  const program = anchor.workspace.Ocr2;
  const accessController = anchor.workspace.AccessController;

  let token: Token, tokenClient: Token;
  let storeAuthority: PublicKey, storeNonce: number;
  let tokenVault: PublicKey, vaultAuthority: PublicKey, vaultNonce: number;

  let oracles = [];
  const f = 2;
  // NOTE: 17 is the most we can fit into one setConfig if we use a different payer
  // if the owner == payer then we can fit 19
  const n = 19; // min: 3 * f + 1;

  // transmits a single round
  let transmit = async (epoch: number, round: number) => {
    let account = await program.account.state.fetch(state.publicKey);

    // Generate and transmit a report
    let report_context = Buffer.alloc(96);
    report_context.set(account.config.latestConfigDigest, 0); // 32 byte config digest
    // 27 byte padding
    report_context.writeUInt32BE(epoch, 32+27); // 4 byte epoch
    report_context.writeUInt8(round, 32+27+4); // 1 byte round
    // 32 byte extra_hash

    const raw_report = Buffer.from([
      97, 91, 43, 83, // observations_timestamp
      7, // observer_count
      0, 1, 2, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // observers
      0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 210, // median
      0, 0, 0, 0, 0, 0, 0, 2, // juels per lamport (2)
    ]);

    let hash = createHash('sha256')
			.update(Buffer.from([raw_report.length]))
      .update(raw_report)
      .update(report_context)
      .digest();

    let raw_signatures = [];
    for (let oracle of oracles.slice(0, f + 1)) { // sign with `f` + 1 oracles
      let { signature, recid } = secp256k1.ecdsaSign(hash, oracle.signer.secretKey);
      raw_signatures.push(...signature);
      raw_signatures.push(recid);
    }

    const transmitter = oracles[0].transmitter;

    const tx = new Transaction();
    tx.add(
      new TransactionInstruction({
        programId: anchor.translateAddress(program.programId),
        keys: [
          { pubkey: state.publicKey, isWritable: true, isSigner: false },
          { pubkey: transmitter.publicKey, isWritable: false, isSigner: true },
          { pubkey: transmissions.publicKey, isWritable: true, isSigner: false },
          { pubkey: workspace.Store.programId, isWritable: false, isSigner: false },
          { pubkey: store.publicKey, isWritable: true, isSigner: false },
          { pubkey: storeAuthority, isWritable: false, isSigner: false },
      ],
      data: Buffer.concat([
          Buffer.from([storeNonce]),
          report_context,
          raw_report,
          Buffer.from(raw_signatures),
        ]),
      })
    );

    try {
      await provider.send(tx, [transmitter]);
    } catch (err) {
      // Translate IDL error
      const idlErrors = anchor.parseIdlErrors(program.idl);
      let translatedErr = ProgramError.parse(err, idlErrors);
      if (translatedErr === null) {
        throw err;
      }
      throw translatedErr;
    }
  }

  it('Funds the payer', async () => {
    await provider.connection.confirmTransaction(
      await provider.connection.requestAirdrop(payer.publicKey, 10000000000),
      "confirmed"
    );
  });

  it('Creates the LINK token', async () => {
    token = await Token.createMint(
      provider.connection,
      payer,
      mintAuthority.publicKey,
      null,
      9, // SPL tokens use a u64, we can fit enough total supply in 9 decimals. Smallest unit is Gjuels
      TOKEN_PROGRAM_ID,
    );

    tokenClient = new Token(
      provider.connection,
      token.publicKey,
      TOKEN_PROGRAM_ID,
      // @ts-ignore
      program.provider.wallet.payer
    );
  });

  it('Creates access controllers', async () => {
    await accessController.rpc.initialize({
      accounts: {
        state: billingAccessController.publicKey,
        payer: provider.wallet.publicKey,
        owner: owner.publicKey,
        rent: SYSVAR_RENT_PUBKEY,
        systemProgram: SystemProgram.programId,
      },
      signers: [billingAccessController],
      preInstructions: [
        await accessController.account.accessController.createInstruction(billingAccessController),
      ],
    });
    await accessController.rpc.initialize({
      accounts: {
        state: requesterAccessController.publicKey,
        payer: provider.wallet.publicKey,
        owner: owner.publicKey,
        rent: SYSVAR_RENT_PUBKEY,
        systemProgram: SystemProgram.programId,
      },
      signers: [requesterAccessController],
      preInstructions: [
        await accessController.account.accessController.createInstruction(requesterAccessController),
      ],
    });
  });

  it('Creates a store', async () => {
    await workspace.Store.rpc.initialize({
      accounts: {
        store: store.publicKey,
        owner: owner.publicKey,
        loweringAccessController: billingAccessController.publicKey,
      },
      signers: [store],
      preInstructions: [
        await workspace.Store.account.store.createInstruction(store),
      ],
    });

  });

  it('Creates the token vault', async () => {
    [vaultAuthority, vaultNonce] = await PublicKey.findProgramAddress(
      [Buffer.from(anchor.utils.bytes.utf8.encode("vault")), state.publicKey.toBuffer()],
      program.programId
    );

    // Create an associated token account for LINK, owned by the program instance
    tokenVault = await Token.getAssociatedTokenAddress(
      ASSOCIATED_TOKEN_PROGRAM_ID,
      TOKEN_PROGRAM_ID,
      token.publicKey,
      vaultAuthority,
      true, // allowOwnerOffCurve: seems required since a PDA isn't a valid keypair
    );
  });

  it('Initializes the OCR2 feed', async () => {
    console.log("Initializing...");

    console.log("state", state.publicKey.toBase58());
    console.log("transmissions", transmissions.publicKey.toBase58());
    console.log("payer", provider.wallet.publicKey.toBase58());
    console.log("owner", owner.publicKey.toBase58());
    console.log("tokenMint", token.publicKey.toBase58());
    console.log("tokenVault", tokenVault.toBase58());
    console.log("vaultAuthority", vaultAuthority.toBase58());
    console.log("placeholder", placeholder.toBase58());
    
    // TODO: I wasn't able to build a createFeed instruction + createInstruction(transmissions) to do an atomic rpc.initialize call
    // let createFeed = store.instruction.createFeed({
    const granularity = 30;
    const liveLength = 1024;
    await workspace.Store.rpc.createFeed(
      granularity,
      liveLength,
    {
      accounts: {
        store: store.publicKey,
        feed: transmissions.publicKey,
        authority: owner.publicKey,
      },
      signers: [transmissions],
      preInstructions: [
        await workspace.Store.account.transmissions.createInstruction(transmissions, 8+128+8096*24),
      ],
    });
    // Program log: panicked at 'range end index 8 out of range for slice of length 0', store/src/lib.rs:476:10

    // Configure threshold for the feed
    await workspace.Store.rpc.setValidatorConfig(flaggingThreshold,
      {
        accounts: {
          store: store.publicKey,
          feed: transmissions.publicKey,
          authority: owner.publicKey,
        },
        signers: [],
    });

    // store authority for our ocr2 config
    [storeAuthority, storeNonce] = await PublicKey.findProgramAddress(
      [Buffer.from(anchor.utils.bytes.utf8.encode("store")), state.publicKey.toBuffer()],
      program.programId
    );
  });
    
  it('Initializes the OCR2 config', async () => {
    await program.rpc.initialize(vaultNonce, new BN(minAnswer), new BN(maxAnswer), decimals, description, {
      accounts: {
        state: state.publicKey,
        transmissions: transmissions.publicKey,
        payer: provider.wallet.publicKey,
        owner: owner.publicKey,
        tokenMint: token.publicKey,
        tokenVault: tokenVault,
        vaultAuthority: vaultAuthority,
        requesterAccessController: requesterAccessController.publicKey,
        billingAccessController: billingAccessController.publicKey,
        rent: SYSVAR_RENT_PUBKEY,
        systemProgram: SystemProgram.programId,
        tokenProgram: TOKEN_PROGRAM_ID,
        associatedTokenProgram: ASSOCIATED_TOKEN_PROGRAM_ID,
      },
      signers: [state],
      preInstructions: [
        await program.account.state.createInstruction(state),
        // await store.account.transmissions.createInstruction(transmissions, 8+128+8096*24),
        // createFeed,
      ],
    });

    let account = await program.account.state.fetch(state.publicKey);
    let config = account.config;
    assert.ok(config.minAnswer.toNumber() == minAnswer);
    assert.ok(config.maxAnswer.toNumber() == maxAnswer);
    assert.ok(config.decimals == 18);

    console.log(`Generating ${n} oracles...`);
    let futures = [];
    let generateOracle = async () => {
      let secretKey = randomBytes(32);
      let transmitter = Keypair.generate();
      return {
        signer: {
          secretKey,
          publicKey: secp256k1.publicKeyCreate(secretKey, false).slice(1), // compressed = false, skip first byte (0x04)
        },
        transmitter,
        // Initialize a token account
        payee: await token.getOrCreateAssociatedAccountInfo(transmitter.publicKey),
      }
    }
    for (let i = 0; i < n; i++) {
      futures.push(generateOracle())
    }
    oracles = await Promise.all(futures);

    const offchain_config_version = 1;
    const offchain_config = Buffer.from([4, 5, 6]);

    // Fund the owner with LINK tokens
    await token.mintTo(
      tokenVault,
      mintAuthority.publicKey,
      [mintAuthority],
      1000000000
    );

    // TODO: listen for SetConfig event

    console.log("beginOffchainConfig");
    await program.rpc.beginOffchainConfig(
      new BN(offchain_config_version),
      {
        accounts: {
          state: state.publicKey,
          authority: owner.publicKey,
        },
    });
    console.log("writeOffchainConfig");
    await program.rpc.writeOffchainConfig(
      offchain_config,
      {
        accounts: {
          state: state.publicKey,
          authority: owner.publicKey,
        },
    });
    console.log("writeOffchainConfig");
    await program.rpc.writeOffchainConfig(
      offchain_config,
      {
        accounts: {
          state: state.publicKey,
          authority: owner.publicKey,
        },
    });
    console.log("commitOffchainConfig");
    await program.rpc.commitOffchainConfig(
      {
        accounts: {
          state: state.publicKey,
          authority: owner.publicKey,
        },
    });
    account = await program.account.state.fetch(state.publicKey);
    config = account.config;
    assert.ok(config.offchainConfig.len == 6);
    assert.deepEqual(config.offchainConfig.xs.slice(0, config.offchainConfig.len), [4,5,6,4,5,6]);

    // Call setConfig
    console.log("setConfig");
    await program.rpc.setConfig(oracles.map((oracle) => ({
        signer: ethereumAddress(Buffer.from(oracle.signer.publicKey)),
        transmitter: oracle.transmitter.publicKey,
      })),
      f,
      {
        accounts: {
          state: state.publicKey,
          authority: owner.publicKey,
        },
        signers: [],
    });
    console.log("setPayees")
    await program.rpc.setPayees(
      oracles.map((oracle) => oracle.payee.address),
      {
        accounts: {
          state: state.publicKey,
          authority: owner.publicKey,
        },
        signers: [],
    });
    console.log("setBilling")
    await program.rpc.setBilling(
      new BN(1),
      new BN(1),
      {
        accounts: {
          state: state.publicKey,
          authority: owner.publicKey,
          accessController: billingAccessController.publicKey,
        },
        signers: [],
    });
  });

  it("Can't transmit a round if not the writer", async () => {
    try {
      await transmit(1, 1);
      assert.fail("transmit() shouldn't have succeeded!");
    } catch {
      // transmit should fail
    }
  });

  it('Sets the cluster as the feed writer', async () => {
    console.log("Adding cluster to feed");
    await workspace.Store.rpc.setWriter(
      storeAuthority,
      {
        accounts: {
          store: store.publicKey,
          feed: transmissions.publicKey,
          authority: owner.publicKey,
        },
      });
  });

  it('Transmits a round', async () => {
    await transmit(1, 2);
    let feed = await provider.connection.getAccountInfo(transmissions.publicKey);
    console.log(feed);
  });

  it('Withdraws funds', async () => {
    const recipient = await token.createAccount(placeholder);
    let recipientTokenAccount = await token.getOrCreateAssociatedAccountInfo(recipient);

    await program.rpc.withdrawFunds(
      new BN(1),
      {
        accounts: {
          state: state.publicKey,
          authority: owner.publicKey,
          accessController: billingAccessController.publicKey,
          tokenVault: tokenVault,
          vaultAuthority: vaultAuthority,
          recipient: recipientTokenAccount.address,
          tokenProgram: TOKEN_PROGRAM_ID,
        },
        signers: [],
      }
    );

    recipientTokenAccount = await tokenClient.getOrCreateAssociatedAccountInfo(recipient);
    assert.ok(recipientTokenAccount.amount.toNumber() === 1);
  });

  it('Calling setConfig again should move payments over to leftover payments', async () => {
    await program.rpc.setConfig(oracles.map((oracle) => ({
        signer: ethereumAddress(Buffer.from(oracle.signer.publicKey)),
        transmitter: oracle.transmitter.publicKey,
      })),
      f,
      {
        accounts: {
          state: state.publicKey,
          authority: owner.publicKey,
        },
        signers: [],
    });
    let account = await program.account.state.fetch(state.publicKey);
    let leftovers = account.leftoverPayments.xs.slice(0, account.leftoverPayments.len);
    for (let leftover of leftovers) {
      assert.ok(leftover.amount.toNumber() !== 0);
    }

    console.log("payRemaining");
    let remaining = leftovers.map((leftover) => { return { pubkey: leftover.payee, isWritable: true, isSigner: false }});

    await program.rpc.payRemaining(
      {
        accounts: {
          state: state.publicKey,
          authority: owner.publicKey,
          accessController: billingAccessController.publicKey,
          tokenVault: tokenVault,
          vaultAuthority: vaultAuthority,
          tokenProgram: TOKEN_PROGRAM_ID,
        },
        remainingAccounts: remaining,
        signers: [],
      }
    );

    account = await program.account.state.fetch(state.publicKey);
    assert.ok(account.leftoverPayments.len == 0);
  });

  it('Can call query', async () => {
    let buffer = Keypair.generate();
    await workspace.Store.rpc.query(
      Scope.Version,
      {
        accounts: {
          feed: transmissions.publicKey,
          buffer: buffer.publicKey,
        },
        preInstructions: [
          SystemProgram.createAccount({
            fromPubkey: provider.wallet.publicKey,
            newAccountPubkey: buffer.publicKey,
            lamports: await provider.connection.getMinimumBalanceForRentExemption(256),
            space: 256,
            programId: workspace.Store.programId,
          })
        ],
        signers: [buffer]
      }
    );

    // let account = await program.account.latestConfig.fetch(buffer.publicKey);
    let account = await workspace.Store.account.version.fetch(buffer.publicKey);
    console.log(account);
  });
});
