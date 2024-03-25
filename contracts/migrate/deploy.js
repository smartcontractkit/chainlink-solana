const anchor = require("@coral-xyz/anchor");
const fs = require("fs");
const BufferLayout = require("@solana/buffer-layout");

let BN = anchor.BN;

const CHAINLINK_PROGRAM_ID = new anchor.web3.PublicKey(
  process.env.CHAINLINK_PROGRAM_ID
);

const UPGRADEABLE_BPF_LOADER_PROGRAM_ID = new anchor.web3.PublicKey(
  "BPFLoaderUpgradeab1e11111111111111111111111"
);

const provider = anchor.AnchorProvider.env();

// Configure the cluster.
anchor.setProvider(provider);

const encodeInstruction = (data) => {
  const CHUNK_SIZE = 900;

  const dataLayout = BufferLayout.union(BufferLayout.u32("tag"), null, "tag");
  dataLayout.addVariant(0, BufferLayout.struct([]), "InitializeBuffer");
  const write = BufferLayout.struct([
    BufferLayout.u32("offset"),
    BufferLayout.nu64("length"),
    BufferLayout.seq(
      BufferLayout.u8("byte"),
      BufferLayout.offset(BufferLayout.u32(), -8),
      "bytes"
    ),
  ]);
  dataLayout.addVariant(1, write, "Write");
  const deployWithMaxLen = BufferLayout.struct([
    BufferLayout.nu64("max_data_len"),
  ]);
  dataLayout.addVariant(2, deployWithMaxLen, "DeployWithMaxDataLen");
  dataLayout.addVariant(3, BufferLayout.struct([]), "Upgrade");
  dataLayout.addVariant(4, BufferLayout.struct([]), "SetAuthority");
  dataLayout.addVariant(5, BufferLayout.struct([]), "Close");

  // UpgradeableLoaderInstruction tag + offset + chunk length + chunk data
  const instructionBuffer = Buffer.alloc(4 + 4 + 8 + CHUNK_SIZE);
  const encodedSize = dataLayout.encode(data, instructionBuffer);
  return instructionBuffer.slice(0, encodedSize);
};

async function main() {
  // #region main
  // Read the generated IDL.
  const idl = JSON.parse(
    fs.readFileSync("examples/hello-world/target/idl/hello_world.json", "utf8")
  );

  // Address of the deployed program.
  const programId = new anchor.web3.PublicKey(
    "Fg6PaFpoGXkYsidMpWTK6W2BeZ7FEfcYkg476zPFsLnS"
  );

  // Generate the program client from IDL.
  const program = new anchor.Program(idl, programId);

  const owner = provider.wallet;
  const store = anchor.web3.Keypair.generate();
  const feed = anchor.web3.Keypair.generate();
  const accessController = anchor.web3.Keypair.generate();

  let storeIdl = JSON.parse(fs.readFileSync("target/idl/store.json"));
  const storeProgram = new anchor.Program(
    storeIdl,
    CHAINLINK_PROGRAM_ID,
    provider
  );

  let acIdl = JSON.parse(fs.readFileSync("target/idl/access_controller.json"));
  const accessControllerProgram = new anchor.Program(
    acIdl,
    "2F5NEkMnCRkmahEAcQfTQcZv1xtGgrWFfjENtTwHLuKg",
    provider
  );

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
      await accessControllerProgram.account.accessController.createInstruction(
        accessController
      ),
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
        await storeProgram.account.transmissions.createInstruction(
          feed,
          8 + 128 + 6 * 24
        ),
      ],
    }
  );

  await storeProgram.rpc.setWriter(owner.publicKey, {
    accounts: {
      store: store.publicKey,
      feed: feed.publicKey,
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

  let header = await storeProgram.account.transmissions.fetch(feed.publicKey);
  console.log(header);

  // -- Upgrade the store program.

  console.log("Upgrading store program...");
  // build deploy buffer instruction
  const [programDataKey, _nonce] =
    await anchor.web3.PublicKey.findProgramAddress(
      [storeProgram.programId.toBuffer()],
      UPGRADEABLE_BPF_LOADER_PROGRAM_ID
    );
  let bufferAccount = new anchor.web3.PublicKey(process.env.BUFFER);
  const upgradeData = encodeInstruction({ Upgrade: {} });
  const upgradeAccounts = [
    { pubkey: programDataKey, isSigner: false, isWritable: true },
    { pubkey: storeProgram.programId, isSigner: false, isWritable: true },
    { pubkey: bufferAccount, isSigner: false, isWritable: true },
    { pubkey: owner.publicKey, isSigner: false, isWritable: true },
    {
      pubkey: anchor.web3.SYSVAR_RENT_PUBKEY,
      isSigner: false,
      isWritable: false,
    },
    {
      pubkey: anchor.web3.SYSVAR_CLOCK_PUBKEY,
      isSigner: false,
      isWritable: false,
    },
    { pubkey: owner.publicKey, isSigner: true, isWritable: false },
  ];

  let transmissionAccounts = [
    {
      pubkey: feed.publicKey,
      isSigner: false,
      isWritable: true,
    },
  ];
  const migrateData = storeProgram.coder.instruction.encode("migrate", {});
  const migrateAccounts = [
    { pubkey: store.publicKey, isSigner: false, isWritable: true },
    { pubkey: owner.publicKey, isSigner: true, isWritable: false },
    ...transmissionAccounts,
  ];

  try {
    const upgradeTx = new anchor.web3.Transaction();
    upgradeTx.add(
      new anchor.web3.TransactionInstruction({
        programId: UPGRADEABLE_BPF_LOADER_PROGRAM_ID,
        keys: upgradeAccounts,
        data: upgradeData,
      })
    );
    upgradeTx.add(
      new anchor.web3.TransactionInstruction({
        data: migrateData,
        keys: migrateAccounts,
        programId: storeProgram.programId,
      })
    );
    await provider.send(upgradeTx);
  } catch (err) {
    // Translate IDL error
    const idlErrors = anchor.parseIdlErrors(storeProgram.idl);
    let translatedErr = anchor.ProgramError.parse(err, idlErrors);
    if (translatedErr === null) {
      throw err;
    }
    throw translatedErr;
  }

  header = await storeProgram.account.transmissions.fetch(feed.publicKey);
  console.log(header);

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

  // #endregion main
}

console.log("Running client...");
main().then(() => console.log("Success"));
