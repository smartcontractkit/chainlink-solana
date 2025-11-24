import * as anchor from "@coral-xyz/anchor";
import { Program, BN } from "@coral-xyz/anchor";
import { KeystoneForwarder } from "../target/types/keystone_forwarder";
import {
  AddressLookupTableProgram,
  ComputeBudgetProgram,
  Keypair,
  PublicKey,
  TransactionMessage,
  VersionedTransaction,
} from "@solana/web3.js";
import { keccak256 } from "ethereum-cryptography/keccak";
import { assert } from "chai";
import { createHash } from "crypto";
import {
  calculateForwarderAuthorityBump,
  encodeForwarderReport,
  generateAccountHash,
  generateEthKeypair,
  signMessage,
  waitForEvent,
} from "./utils";
import chaiAsPromised from "chai-as-promised";
import { DummyReceiver } from "../target/types/dummy_receiver";

// chai.use(chaiAsPromised);

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

// Helper function to parse oracle config account data
function parseOraclesConfigAccount(data: Buffer) {
  // Layout: discriminator(8) + config_id(8) + f(1) + padding(7) + signer_addresses
  // SignerAddresses layout: xs(32*20) + len(1) + padding(7)

  let offset = 8; // Skip discriminator

  // Read config_id (8 bytes, little-endian)
  const configId = data.readBigUInt64LE(offset);
  offset += 8;

  // Read f (1 byte)
  const f = data.readUInt8(offset);
  offset += 1;

  // Skip padding (7 bytes)
  offset += 7;

  // Read SignerAddresses structure
  // Layout: xs (32*20 bytes) + len (1 byte) + padding (7 bytes)
  const signerAddressesLen = data.readUInt8(offset + 32 * 20); // len is after xs array

  // Extract the actual addresses from xs array
  const signerAddresses = [];
  for (let i = 0; i < signerAddressesLen; i++) {
    const addressOffset = offset + i * 20;
    const address = data.slice(addressOffset, addressOffset + 20);
    signerAddresses.push(address);
  }

  return {
    configId,
    f,
    signerAddresses,
  };
}

// Helper function to get and parse oracle config account
async function getOraclesConfigAccount(
  program: any,
  oraclesConfigStorage: PublicKey
) {
  const accountInfo = await program.provider.connection.getAccountInfo(
    oraclesConfigStorage
  );
  if (!accountInfo) {
    throw new Error("Account not found");
  }

  return parseOraclesConfigAccount(accountInfo.data);
}

let getEthereumAddress = (publicKey: Buffer) => {
  return keccak256(publicKey).slice(12);
};

describe("keystone_storage", function () {
  this.timeout(30_000);
  // Configure the client to use the local cluster.
  const provider = anchor.AnchorProvider.env();
  anchor.setProvider(provider);

  const program = anchor.workspace
    .KeystoneForwarder as Program<KeystoneForwarder>;

  const receiverProgram = anchor.workspace
    .DummyReceiver as Program<DummyReceiver>;
  const latestReportState = Keypair.generate();

  // in BFT algorithm with N nodes, the maximum # of faulty nodes you can stomach is f where
  // f = floor(N / 3)
  //
  // conversely, if you decide to support f faulty nodes, then
  // the DON size you can need to ensure BFT is N >= 3*f + 1,
  // where we choose N = 3*f + 1 for convinience.

  // with an f chosen, we need at elast f + 1 signatures to verify in the "report",

  // if f + 1 = 6 , then N >= 16
  // if f + 1 = 7, then N >= 19

  const N = 16;
  const f = Math.floor(N / 3);

  // forwarder state data account
  const forwarderState = Keypair.generate();

  let defaultOraclesConfigStorage: anchor.web3.PublicKey;
  const defaultSigners = Array.from({ length: N }, () => generateEthKeypair());
  defaultSigners.sort((a, b) => {
    return Buffer.compare(a.ethereumAddress, b.ethereumAddress);
  });

  it("Is initialized!", async () => {
    const eventPromise = waitForEvent(
      program,
      "ForwarderInitialize",
      (event: any, slot) => {
        assert.isNotNull(event.authorityNonce);
        assert.isTrue(
          event.owner.equals(provider.wallet.publicKey),
          "Owner set"
        );
      }
    );

    await program.methods
      .initialize()
      .accounts({
        state: forwarderState.publicKey,
        owner: provider.wallet.publicKey,
        systemProgram: anchor.web3.SystemProgram.programId,
      })
      .signers([forwarderState])
      .rpc();

    const actualState = await program.account.forwarderState.fetch(
      forwarderState.publicKey
    );

    assert.isTrue(
      actualState.owner.equals(provider.wallet.publicKey),
      "owner set"
    );
    assert.isTrue(
      actualState.proposedOwner.equals(PublicKey.default),
      "proposed owner is 0"
    );
    assert.equal(actualState.version, 1, "version 1");

    await eventPromise;
  });

  it("Transfer ownership and back", async () => {
    const proposedOwner = Keypair.generate();

    // transfer ownership event emitted
    // const transferOwnershipEventPromise = waitForEvent(
    //   program,
    //   "OwnershipTransfer",
    //   (event: any, slot) => {
    //     assert.isTrue(
    //       event.proposedOwner.equals(proposedOwner.publicKey),
    //       "proposed owner key emitted"
    //     );
    //     assert.isTrue(
    //       event.currentOwner.equals(provider.wallet.publicKey),
    //       "current owner key emitted"
    //     );
    //   }
    // );

    // current owner initiates transfer to proposed owner
    await program.methods
      .transferOwnership(proposedOwner.publicKey)
      .accounts({
        state: forwarderState.publicKey,
        currentOwner: provider.wallet.publicKey,
      })
      .rpc();

    const actualState1 = await program.account.forwarderState.fetch(
      forwarderState.publicKey
    );

    assert.isTrue(
      actualState1.owner.equals(provider.wallet.publicKey),
      "owner should be same"
    );
    assert.isTrue(
      actualState1.proposedOwner.equals(proposedOwner.publicKey),
      "proposed owner set"
    );

    // console.log("transfer");
    // await transferOwnershipEventPromise;

    // const acceptOwnershipEventPromise = waitForEvent(
    //   program,
    //   "OwnershipAcceptance",
    //   (event: any, slot) => {
    //     assert.isTrue(
    //       event.newOwner.equals(proposedOwner.publicKey),
    //       "new owner key emitted"
    //     );
    //     assert.isTrue(
    //       event.previousOwner.equals(provider.wallet.publicKey),
    //       "previous owner key emitted"
    //     );
    //   }
    // );

    // proposed owner accepts
    await program.methods
      .acceptOwnership()
      .accounts({
        state: forwarderState.publicKey,
        proposedOwner: proposedOwner.publicKey,
      })
      .signers([proposedOwner])
      .rpc();

    const actualState2 = await program.account.forwarderState.fetch(
      forwarderState.publicKey
    );

    assert.isTrue(
      actualState2.owner.equals(proposedOwner.publicKey),
      "owner set correctly"
    );
    assert.isTrue(
      actualState2.proposedOwner.equals(PublicKey.default),
      "proposed owner is 0"
    );

    // proposed owner transfer back
    // current owner initiates transfer to proposed owner
    await program.methods
      .transferOwnership(provider.wallet.publicKey)
      .accounts({
        state: forwarderState.publicKey,
        currentOwner: proposedOwner.publicKey,
      })
      .signers([proposedOwner])
      .rpc();

    // console.log("accept");
    // // place after another call in order to flush events
    // await acceptOwnershipEventPromise;

    const actualState3 = await program.account.forwarderState.fetch(
      forwarderState.publicKey
    );

    assert.isTrue(
      actualState3.owner.equals(proposedOwner.publicKey),
      "owner should be same"
    );
    assert.isTrue(
      actualState3.proposedOwner.equals(provider.wallet.publicKey),
      "proposed owner set"
    );

    // proposed owner accepts
    await program.methods
      .acceptOwnership()
      .accounts({
        state: forwarderState.publicKey,
        proposedOwner: provider.wallet.publicKey,
      })
      .rpc();

    const actualState4 = await program.account.forwarderState.fetch(
      forwarderState.publicKey
    );

    assert.isTrue(
      actualState4.owner.equals(provider.wallet.publicKey),
      "owner set correctly"
    );
    assert.isTrue(
      actualState4.proposedOwner.equals(PublicKey.default),
      "proposed owner is 0"
    );
  });

  it("Initialize New Oracles Config, Update", async () => {
    const donId = 7;
    const configVersion = 3;
    const configId: bigint = (7n << 32n) | 3n; // 64 bytes

    const configIdBytes = Buffer.alloc(8);
    configIdBytes.writeBigUInt64BE(configId);

    const seeds = [
      Buffer.from(anchor.utils.bytes.utf8.encode("config")),
      forwarderState.publicKey.toBuffer(),
      configIdBytes,
    ];

    const [oraclesConfigStorage, _bump] = PublicKey.findProgramAddressSync(
      seeds,
      program.programId
    );
    defaultOraclesConfigStorage = oraclesConfigStorage;

    const signers = defaultSigners;

    // all 3 work... arrays of number[], Uint8Array, or Buffer
    // const signerEthAddresses = signers.map(s => Array.from(s.ethereumAddress).map(x => new BN(x)));
    // const signerEthAddresses = signers.map(s => ( new Uint8Array(s.ethereumAddress) ))
    const signerEthAddresses = signers.map((s) => s.ethereumAddress);

    const initialEthAddresses = signerEthAddresses.slice(0, 4);

    // f = 1, N = 4 initially
    await program.methods
      .initOraclesConfig(
        new BN(7),
        new BN(3),
        new BN(1),
        initialEthAddresses as any
      )
      .accounts({
        state: forwarderState.publicKey,
        oraclesConfig: oraclesConfigStorage,
        owner: provider.wallet.publicKey,
        systemProgram: anchor.web3.SystemProgram.programId,
      })
      .rpc();

    // const actualConfig = await program.account.oraclesConfig.fetch(
    //   oraclesConfigStorage
    // );

    // Parse initial config using helper function
    const initialConfig = await getOraclesConfigAccount(
      program,
      oraclesConfigStorage
    );

    assert.equal(configId, initialConfig.configId, "config ids should equal");
    assert.equal(1, initialConfig.f, "f should equal");
    assert.equal(initialConfig.signerAddresses.length, 4, "4 signer addresses");
    for (let i = 0; i < initialConfig.signerAddresses.length; i++) {
      assert.isTrue(
        initialConfig.signerAddresses[i].equals(
          Buffer.from(initialEthAddresses[i])
        ),
        `Signer address ${i} should match`
      );
    }

    const configPromise = waitForEvent(
      program,
      "ConfigSet",
      (event: any, slot) => {
        assert.isTrue(event.donId === 7, "don Id set");
        assert.isTrue(event.f === f, "f is equal");
        assert.isNotNull(event.signers, "signers not null");
        assert.isTrue(event.configVersion === 3, "config version is 3");
      }
    );

    // update to new f
    await program.methods
      .updateOraclesConfig(
        new BN(7),
        new BN(3),
        new BN(f),
        signerEthAddresses as any
      )
      .accounts({
        state: forwarderState.publicKey,
        oraclesConfig: oraclesConfigStorage,
        owner: provider.wallet.publicKey,
      })
      .rpc();

    // const actualUpdatedConfig = await program.account.oraclesConfig.fetch(
    //   oraclesConfigStorage
    // );

    // Parse updated config using helper function
    const updatedConfig = await getOraclesConfigAccount(
      program,
      oraclesConfigStorage
    );

    await configPromise;

    assert.equal(
      Number(configId),
      Number(updatedConfig.configId),
      "config ids should equal"
    );
    assert.equal(f, updatedConfig.f, "f should equal");

    for (let i = 0; i < updatedConfig.signerAddresses.length; i++) {
      assert.isTrue(
        updatedConfig.signerAddresses[i].equals(
          Buffer.from(signerEthAddresses[i])
        ),
        `Updated signer address ${i} should match`
      );
    }
  });

  it("Close oracle config", async () => {
    const donId = 9n;
    const configVersion = 2n;
    const configId: bigint = (donId << 32n) | configVersion;

    const configIdBytes = Buffer.alloc(8);
    configIdBytes.writeBigUInt64BE(configId);

    const seeds = [
      Buffer.from(anchor.utils.bytes.utf8.encode("config")),
      forwarderState.publicKey.toBuffer(),
      configIdBytes,
    ];

    const [oraclesConfigStorage, _bump] = PublicKey.findProgramAddressSync(
      seeds,
      program.programId
    );

    const signers = Array.from({ length: 16 }, () => generateEthKeypair());
    signers.sort((a, b) => {
      return Buffer.compare(a.ethereumAddress, b.ethereumAddress);
    });

    const signerEthAddresses = signers.map((s) => s.ethereumAddress);

    const ix = await program.methods
      .initOraclesConfig(
        new BN(9),
        new BN(2),
        new BN(1),
        signerEthAddresses as any
      )
      .accounts({
        state: forwarderState.publicKey,
        oraclesConfig: oraclesConfigStorage,
        owner: provider.wallet.publicKey,
        systemProgram: anchor.web3.SystemProgram.programId,
      })
      .instruction();

    const message = new TransactionMessage({
      payerKey: provider.wallet.publicKey, // Account paying for the transaction
      recentBlockhash: (await provider.connection.getLatestBlockhash())
        .blockhash, // Latest blockhash
      instructions: [ix], // Instructions to be included in the transaction
    }).compileToV0Message();

    const tx = new VersionedTransaction(message);

    const signedTx = await provider.wallet.signTransaction(tx);

    const serializedTx = signedTx.serialize();

    // console.log(`tx size w/ 17 nodes ${serializedTx.length}`);

    await provider.sendAndConfirm(signedTx);

    const actualConfig = await program.account.oraclesConfig.fetch(
      oraclesConfigStorage
    );

    assert.equal(configId, actualConfig.configId, "config ids should equal");

    await program.methods
      .closeOraclesConfig(new BN(9), new BN(2))
      .accounts({
        state: forwarderState.publicKey,
        oraclesConfig: oraclesConfigStorage,
        owner: provider.wallet.publicKey,
      })
      .rpc();

    try {
      await program.account.oraclesConfig.fetch(oraclesConfigStorage);
      assert.fail("Account should not exist anymore");
    } catch (err) {
      if (!err.message.includes("Account does not exist")) {
        assert.fail("Account should not exist anymore");
      }
    }
  });

  it("Report", async () => {
    // use dummy receiver from setup
    const receiver = receiverProgram.programId;

    let workflowExecutionId = 20;
    let reportId = 11;

    const workflowExecutionIdBytes = Buffer.alloc(32);
    workflowExecutionIdBytes.writeUint8(workflowExecutionId, 31); // write at last byte for BigEndian

    const reportIdBytes = Buffer.alloc(2);
    reportIdBytes.writeUint16BE(reportId);

    const transmissionIdBytes = createHash("sha256")
      .update(
        Buffer.concat([
          receiver.toBuffer(),
          workflowExecutionIdBytes,
          reportIdBytes,
        ])
      )
      .digest();

    const [forwarderAuthorityStorage, _] = calculateForwarderAuthorityBump(
      forwarderState.publicKey,
      receiver,
      program.programId
    );

    // begin initializing the receiver program

    await receiverProgram.methods
      .initialize()
      .accounts({
        reportState: latestReportState.publicKey,
        signer: provider.wallet.publicKey,
        systemProgram: anchor.web3.SystemProgram.programId,
        forwarderAuthority: forwarderAuthorityStorage,
      })
      .signers([latestReportState])
      .rpc();

    // finish initializing the receiver program

    const [executionStateStorage, executionStateBump] =
      PublicKey.findProgramAddressSync(
        [
          Buffer.from(anchor.utils.bytes.utf8.encode("execution_state")),
          forwarderState.publicKey.toBuffer(),
          transmissionIdBytes,
        ],
        program.programId
      );

    // data = len_signatures (1) | signatures (N*65) | raw_report (M) | report_context (96)

    // signers for the report only need to be f + 1
    const signers = defaultSigners.slice(0, f + 1);

    const lenSignatureBytes = Buffer.alloc(1);
    lenSignatureBytes.writeUint8(signers.length);

    // generate account hash
    const accountHash = generateAccountHash([
      forwarderState.publicKey.toBuffer(),
      forwarderAuthorityStorage.toBuffer(),
      latestReportState.publicKey.toBuffer(),
    ]);

    const forwarderReportBuffer = encodeForwarderReport(
      accountHash,
      Buffer.from([255])
    );

    // metadata length + actual report payload length
    const rawReportBytes = Buffer.alloc(109 + forwarderReportBuffer.length);

    // version                offset   0, size  1
    // workflow_execution_id  offset   1, size 32
    // timestamp              offset  33, size  4
    // don_id                 offset  37, size  4
    // don_config_version     offset  41, size  4
    // workflow_cid           offset  45, size 32
    // workflow_name          offset  77, size 10
    // workflow_owner         offset  87, size 20
    // report_id              offset 107, size  2

    const version = 1;
    const timestamp = 5;
    const donId = 7;
    const configVersion = 3;
    const workflowCid = 2;
    const workflowName = 10;
    const workflowOwner = 11;

    rawReportBytes.writeUint8(version, 0);
    rawReportBytes.writeUint8(workflowExecutionId, 32); // write at last byte for BigEndian
    rawReportBytes.writeUint8(timestamp, 36);
    rawReportBytes.writeUint8(donId, 40);
    rawReportBytes.writeUint8(configVersion, 44);
    rawReportBytes.writeUint8(workflowCid, 76);
    rawReportBytes.writeUint8(workflowName, 86);
    rawReportBytes.writeUint8(workflowOwner, 106);
    rawReportBytes.writeUint8(reportId, 108);

    // copies forwarderReportBytes into rawReportBytes
    forwarderReportBuffer.copy(rawReportBytes, 109);

    // just keep this zero-ed since we don't use it outside of the hash
    const reportContextBytes = Buffer.alloc(96);

    // the msg to sign includes the prefix of u8(len(rawReportBytes))
    const rawReportLenU8 = Buffer.alloc(1);
    rawReportLenU8.writeUint8(rawReportBytes.length & 0xff);

    console.log("raw report length", rawReportBytes.length);

    const msgHashToSign = createHash("sha256")
      .update(
        Buffer.concat([rawReportLenU8, rawReportBytes, reportContextBytes])
      )
      .digest();

    const signaturesInfo = await Promise.all(
      signers.map((s) => signMessage(msgHashToSign, s.secretKey))
    );

    const signaturesBytes = signaturesInfo.map((s) => {
      const recoveryIdBytes = Buffer.alloc(1);
      recoveryIdBytes.writeUint8(s.recovery);
      return Buffer.concat([s.signature, recoveryIdBytes]);
    });

    // they need to be packed as one buffer array
    const signaturesBytesPacked = Buffer.concat(signaturesBytes);

    // data = len_signatures (1) | signatures (N*65) | raw_report (M) | report_context (96)
    //  1 + (15*65) + 110 + 96
    const dataBytes = Buffer.concat([
      lenSignatureBytes,
      signaturesBytesPacked,
      rawReportBytes,
      reportContextBytes,
    ]);

    const computeLimitIx = ComputeBudgetProgram.setComputeUnitLimit({
      units: 1_400_000,
    });

    const reportPromise = waitForEvent(
      program,
      "ReportProcessed",
      (event: any, slot) => {
        // console.log(event);
        assert.isTrue(
          receiver.equals(event.receiver),
          "receiver pub key emitted"
        );
        assert.isTrue(event.result, "Successful report emitted");
        assert.isNotNull(event.transmissionId, "Transmission Id present");
      }
    );

    const ix = await program.methods
      .report(dataBytes)
      .accounts({
        state: forwarderState.publicKey,
        oraclesConfig: defaultOraclesConfigStorage,
        transmitter: provider.wallet.publicKey,
        forwarderAuthority: forwarderAuthorityStorage,
        executionState: executionStateStorage,
        receiverProgram: receiver,
        systemProgram: anchor.web3.SystemProgram.programId,
      })
      .remainingAccounts([
        {
          pubkey: latestReportState.publicKey,
          isSigner: false,
          isWritable: true,
        },
      ])
      .instruction();

    // await reportPromise;

    const slot = await provider.connection.getSlot();
    const [lookupTableInst, lookupTableAddress] =
      AddressLookupTableProgram.createLookupTable({
        authority: provider.wallet.publicKey,
        payer: provider.wallet.publicKey,
        recentSlot: slot - 1,
      });

    // for chainlink usage, the ALT does include the receiver (bc that is known and static to us)
    // PLUS any data accounts we use in the receiver (bc that is known and static to us)
    // for external usage, the external user can provide another ALT should they wish
    const extendInstruction = AddressLookupTableProgram.extendLookupTable({
      payer: provider.wallet.publicKey,
      authority: provider.wallet.publicKey,
      lookupTable: lookupTableAddress,
      addresses: [
        forwarderState.publicKey,
        defaultOraclesConfigStorage,
        forwarderAuthorityStorage,
        receiver,
        anchor.web3.SystemProgram.programId,
        latestReportState.publicKey,
      ],
    });

    // Create the transaction message
    const message = new TransactionMessage({
      payerKey: provider.wallet.publicKey, // Account paying for the transaction
      recentBlockhash: (await provider.connection.getLatestBlockhash())
        .blockhash, // Latest blockhash
      instructions: [lookupTableInst, extendInstruction], // Instructions to be included in the transaction
    }).compileToV0Message();

    const lookUpTx = new VersionedTransaction(message);

    await provider.wallet.signTransaction(lookUpTx);
    await provider.sendAndConfirm(lookUpTx);

    const lookupTableAccount = (
      await provider.connection.getAddressLookupTable(lookupTableAddress)
    ).value;

    assert.isTrue(
      lookupTableAccount.key.equals(lookupTableAddress),
      "lookup addresses equal"
    );
    assert.equal(
      lookupTableAccount.state.addresses.length,
      6,
      "6 addresses in lookup table"
    );

    const [
      lookupState,
      lookupConfig,
      lookupAuthority,
      lookupReceiver,
      lookupSystemP,
      lookupReceiverReport,
    ] = lookupTableAccount.state.addresses;

    assert.isTrue(
      lookupState.equals(forwarderState.publicKey),
      "forwarder state in lookup table"
    );
    assert.isTrue(
      lookupConfig.equals(defaultOraclesConfigStorage),
      "forwarder state in lookup table"
    );
    assert.isTrue(
      lookupAuthority.equals(forwarderAuthorityStorage),
      "forwarder state in lookup table"
    );
    assert.isTrue(
      lookupReceiver.equals(receiver),
      "forwarder state in lookup table"
    );
    assert.isTrue(
      lookupSystemP.equals(anchor.web3.SystemProgram.programId),
      "forwarder state in lookup table"
    );
    assert.isTrue(
      lookupReceiverReport.equals(latestReportState.publicKey),
      "receiver report state in lookup table"
    );

    // create transaction

    const reportMessage = new TransactionMessage({
      payerKey: provider.wallet.publicKey, // Account paying for the transaction
      recentBlockhash: (await provider.connection.getLatestBlockhash())
        .blockhash, // Latest blockhash
      instructions: [computeLimitIx, ix], // Instructions to be included in the transaction
    }).compileToV0Message([lookupTableAccount]);

    const tx = new VersionedTransaction(reportMessage);

    const signedTx = await provider.wallet.signTransaction(tx);

    const txSerial = signedTx.serialize();
    console.log(`"report" tx size w/ ${f + 1} signers`, txSerial.length);

    // delay in order to activate lookup table
    await sleep(3000);

    await provider.sendAndConfirm(tx);

    const actualExecutionState = await program.account.executionState.fetch(
      executionStateStorage
    );

    assert.equal(
      true,
      actualExecutionState.success,
      "execution should succeed"
    );
    assert.isTrue(
      provider.wallet.publicKey.equals(actualExecutionState.transmitter),
      "expected transmitter"
    );
    assert.deepEqual(
      Array.from(transmissionIdBytes),
      actualExecutionState.transmissionId,
      "expected transmissionid"
    );

    const { metadata: actualMetadata, report: actualReport } =
      await receiverProgram.account.latestReport.fetch(
        latestReportState.publicKey
      );
    assert.deepEqual(Buffer.from([255]), actualReport, "reports match");
    assert.deepEqual(
      rawReportBytes.slice(45, 109),
      actualMetadata,
      "metadatas match"
    );

    const sameReportMessage = new TransactionMessage({
      payerKey: provider.wallet.publicKey, // Account paying for the transaction
      recentBlockhash: (await provider.connection.getLatestBlockhash())
        .blockhash, // Latest blockhash
      instructions: [computeLimitIx, ix], // Instructions to be included in the transaction
    }).compileToV0Message([lookupTableAccount]);

    const sameTx = new VersionedTransaction(sameReportMessage);

    try {
      await provider.sendAndConfirm(sameTx);
      assert.fail(
        `Executing twice should fail with ExecutionAlreadySucceded revert`
      );
    } catch (err) {
      if (!err.message.includes("ExecutionAlreadySucceded")) {
        assert.fail(`Unexpected error: ${err.message}`);
      }
    }
  });
});
