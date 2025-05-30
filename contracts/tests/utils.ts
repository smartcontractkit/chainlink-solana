
import * as anchor from "@coral-xyz/anchor";
import { Program, BN } from "@coral-xyz/anchor";
import { KeystoneForwarder } from "../target/types/keystone_forwarder";
import {
  AccountMeta,
  ComputeBudgetProgram,
  PublicKey,
  TransactionMessage,
  VersionedTransaction,
} from "@solana/web3.js";
import { keccak256 } from "ethereum-cryptography/keccak";
import { randomBytes, createHash } from "crypto";
import * as secp256k1 from "secp256k1";

// initializes it, sets oracle config, and returns enough information to create a message

export function signMessage(message: Buffer, secretKey: Buffer) {
    const { signature, recid: recovery } = secp256k1.ecdsaSign(
      message,
      secretKey
    );
    return {
      signature: Buffer.from(signature),
      recovery, // useful for pubkey recovery
    };
  }

export type EthKeypairInfo = {
    secretKey: Buffer,
    publicKey: Uint8Array<ArrayBuffer>,
    ethereumAddress: Buffer
};

export const generateEthKeypair = () => {
    let secretKey = randomBytes(32);
    let publicKey = secp256k1.publicKeyCreate(secretKey, false).slice(1);
    let ethereumAddress = getEthereumAddress(Buffer.from(publicKey));
    return {
      secretKey,
      publicKey,
      ethereumAddress,
    };
  };

export const getEthereumAddress = (publicKey: Buffer) => {
    return keccak256(publicKey).slice(12);
};

// to be used by other program tests for forwarding data
export class Forwarder {
  public f: number
  public state: anchor.web3.Keypair
  public oracles: Array<EthKeypairInfo>
  public donId: number
  public configVersion: number
  public configId: bigint
  public oraclesConfig: [PublicKey, number]
  public forwarderAuthority: [PublicKey, number]
  public nextReportInfo: { workflowExecutionId: number,  reportId: number }

  constructor(public forwarderProgram:  Program<KeystoneForwarder>, public provider: anchor.AnchorProvider) {
    this.nextReportInfo = {
      workflowExecutionId: 1,
      reportId: 1
    };
  }

  public withOracles(f: number, donId: number, configVersion: number) {
    this.f = f;
    // number of oracles is 3*f + 1 for BFT checks
    this.oracles = Array.from({ length: 3*this.f + 1 }, () => generateEthKeypair());
    this.oracles.sort((a, b) => {
      return Buffer.compare(a.ethereumAddress, b.ethereumAddress);
    });

    this.donId = donId;
    this.configVersion = configVersion;
    this.configId = (BigInt(this.donId) << 32n) | BigInt(this.configVersion);

    const configIdBytes = Buffer.alloc(8);
    configIdBytes.writeBigUInt64BE(this.configId);

    const seeds = [
      Buffer.from(anchor.utils.bytes.utf8.encode("config")),
      this.state.publicKey.toBuffer(),
      configIdBytes,
    ];

    this.oraclesConfig = PublicKey.findProgramAddressSync(
      seeds,
      this.forwarderProgram.programId
    );

    return this;
  }

  public withState(state: anchor.web3.Keypair) {
    this.state = state;

    return this;
  }

  public async initialize(ownerKeypair?: anchor.web3.Keypair) {
    // owner defaults to provider
    const ownerPublicKey = ownerKeypair ? ownerKeypair.publicKey : this.provider.wallet.publicKey;
    const signers = ownerKeypair ? [this.state, ownerKeypair] : [this.state];

    this.forwarderAuthority = PublicKey.findProgramAddressSync(
      [
        Buffer.from(anchor.utils.bytes.utf8.encode("forwarder")),
        this.state.publicKey.toBuffer(),
      ],
      this.forwarderProgram.programId
    );

    return await this.forwarderProgram.methods
    .initialize()
    .accounts({
      state: this.state.publicKey,
      owner: ownerPublicKey,
      systemProgram: anchor.web3.SystemProgram.programId,
    })
    .signers(signers)
    .rpc();

  }

  public async initOraclesConfig(ownerKeypair?: anchor.web3.Keypair) {
    const oracleSigners = this.oracles.map((o) => o.ethereumAddress);

    // owner defaults to provider
    const ownerPublicKey = ownerKeypair ? ownerKeypair.publicKey : this.provider.wallet.publicKey;
    const signers = ownerKeypair ? [ownerKeypair] : [];

    return this.forwarderProgram.methods
    .initOraclesConfig(
      new BN(this.donId),
      new BN(this.configVersion),
      new BN(this.f),
      oracleSigners as any
    )
    .accounts({
      state: this.state.publicKey,
      oraclesConfig: this.oraclesConfig[0],
      owner: ownerPublicKey,
      systemProgram: anchor.web3.SystemProgram.programId,
    })
    .signers(signers)
    .rpc();
  }

  private generateForwarderReport(payload: Buffer, workflowName: Buffer, workflowOwner: Buffer) {
    if (workflowName.length != 10 ) {
      throw Error(`Workflow Name should be 10 bytes. Found ${workflowName.length} bytes`);
    }

    if (workflowOwner.length != 20 ) {
      throw Error(`Workflow Owner should be 20 bytes. Found ${workflowOwner.length} bytes`);
    }

    const { workflowExecutionId, reportId } = this.nextReportInfo 

    // signers for the report only need to be f + 1
    const reportSigners = this.oracles.slice(0, this.f + 1);

    const lenSignatureBytes = Buffer.alloc(1);
    lenSignatureBytes.writeUint8(reportSigners.length);

    // metadata length + actual report payload length
    const rawReportBytes = Buffer.alloc(109 + payload.length);

    // version                offset   0, size  1
    // workflow_execution_id  offset   1, size 32
    // timestamp              offset  33, size  4
    // don_id                 offset  37, size  4
    // don_config_version     offset  41, size  4
    // workflow_cid           offset  45, size 32
    // workflow_name          offset  77, size 10
    // workflow_owner         offset  87, size 20
    // report_id              offset 107, size  2

    // it's ok to hardcode some of these values...

    const version = 1;
    const timestamp = 5;
    const donId = this.donId;
    const configVersion = this.configVersion;
    const workflowCid = 2;
    // const workflowName = 10;
    // const workflowOwner = 11;

    rawReportBytes.writeUint8(version, 0);
    rawReportBytes.writeUint8(workflowExecutionId, 32); // write at last byte for BigEndian
    rawReportBytes.writeUint8(timestamp, 36);
    rawReportBytes.writeUint8(donId, 40);
    rawReportBytes.writeUint8(configVersion, 44);
    rawReportBytes.writeUint8(workflowCid, 76);
    workflowName.copy(rawReportBytes, 77);
    // rawReportBytes.writeUint8(workflowName, 86);
    workflowOwner.copy(rawReportBytes, 87);
    // rawReportBytes.writeUint8(workflowOwner, 106);
    rawReportBytes.writeUint8(reportId, 108);

    payload.copy(rawReportBytes, 109)

    // just keep this zero-ed since we don't use it outside of the hash
    const reportContextBytes = Buffer.alloc(96);

    const msgHashToSign = createHash("sha256")
      .update(Buffer.concat([rawReportBytes, reportContextBytes]))
      .digest();

    const signaturesInfo = reportSigners.map((s) =>
      signMessage(msgHashToSign, s.secretKey)
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
    return Buffer.concat([
      lenSignatureBytes,
      signaturesBytesPacked,
      rawReportBytes,
      reportContextBytes,
    ]);
  }

  public async report(receiverProgram: anchor.web3.PublicKey, payload: Buffer,  workflowName: Buffer, workflowOwner: Buffer, remainingAccounts: Array<AccountMeta>
  ) {

    const { workflowExecutionId, reportId } = this.nextReportInfo 
   
    const workflowExecutionIdBytes = Buffer.alloc(32);
    workflowExecutionIdBytes.writeUint32BE(workflowExecutionId, 28); // write at 4 bytes (32 bits)

    const reportIdBytes = Buffer.alloc(2);
    reportIdBytes.writeUint16BE(reportId);

    const transmissionIdBytes = createHash("sha256")
    .update(
      Buffer.concat([
        receiverProgram.toBuffer(),
        workflowExecutionIdBytes,
        reportIdBytes,
      ])
    )
    .digest();

    // to return later
    const [executionStateStorage, executionStateBump] = PublicKey.findProgramAddressSync(
      [
        Buffer.from(anchor.utils.bytes.utf8.encode("execution_state")),
        this.state.publicKey.toBuffer(),
        transmissionIdBytes,
      ],
      this.forwarderProgram.programId
    );

    // increment this.nextReportInfo no matter what

    const dataBytes = this.generateForwarderReport(payload, workflowName, workflowOwner);

    // todo: add lookup table in future for more accurate results?
    const ix = await this.forwarderProgram.methods
    .report(dataBytes)
    .accounts({
      state: this.state.publicKey,
      oraclesConfig: this.oraclesConfig[0],
      transmitter: this.provider.wallet.publicKey, // not used for anything besides payment
      forwarderAuthority: this.forwarderAuthority[0],
      executionState: executionStateStorage,
      receiverProgram: receiverProgram,
      systemProgram: anchor.web3.SystemProgram.programId,
    })
    .remainingAccounts(remainingAccounts)
    .instruction();

    const computeLimitIx = ComputeBudgetProgram.setComputeUnitLimit({
      units: 1_400_000,
    });

    const reportMessage = new TransactionMessage({
      payerKey: this.provider.wallet.publicKey, // Account paying for the transaction
      recentBlockhash: (await this.provider.connection.getLatestBlockhash())
        .blockhash, // Latest blockhash
      instructions: [computeLimitIx, ix], // Instructions to be included in the transaction
    }).compileToV0Message();


    const tx = new VersionedTransaction(reportMessage);

    const signedTx = await this.provider.wallet.signTransaction(tx);

    const txResult = await this.provider.sendAndConfirm(signedTx);

    // increments so it avoids reverting due to reporting the "same" transmission sucessfully
    this.nextReportInfo.reportId++;
    this.nextReportInfo.workflowExecutionId++

    return {
      result: txResult, 
      executionState: [executionStateStorage, executionStateBump]
    }

  }

}
