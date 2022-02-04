import { Result } from '@chainlink/gauntlet-core'
import { Proto, sharedSecretEncryptions } from '@chainlink/gauntlet-core/dist/crypto'
import { logger, prompt, BN } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse, RawTransaction } from '@chainlink/gauntlet-solana'
import { AccountMeta, PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'
import { getRDD } from '../../../../lib/rdd'
import WriteOffchainConfig, { Input } from './write'
import { descriptor as OCR2Descriptor } from '../../../../lib/ocr2Proto'
import { join } from 'path'

export default class CommitOffchainConfig extends SolanaCommand {
  static id = 'ocr2:commit_offchain_config'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:commit_offchain_config --network=devnet --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC --rdd=<PATH_TO_RDD>',
    'yarn gauntlet ocr2:commit_offchain_config --network=devnet --state=5oMNhuuRmxPGEk8ymvzJRAJFJGs7jaHsaxQ3Q2m6PVTR EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]
  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.state, 'Please provide flags with "state"')
  }

  makeInput = (userInput: any): Input => {
    if (userInput) return userInput as Input
    const rdd = getRDD(this.flags.rdd)
    return WriteOffchainConfig.makeInputFromRDD(rdd, this.flags.state)
  }

  serializeOffchainConfig = async (
    input: Input,
    secret?: string,
  ): Promise<{ offchainConfig: Buffer; randomSecret: string }> => {
    const { configPublicKeys, ...validInput } = input
    const proto = new Proto.Protobuf({ descriptor: OCR2Descriptor })
    const reportingPluginConfigProto = proto.encode(
      'offchainreporting2_config.ReportingPluginConfig',
      validInput.reportingPluginConfig,
    )
    const { sharedSecretEncryptions, randomSecret } = await this.generateSecretEncryptions(configPublicKeys, secret)
    const offchainConfig = {
      ...validInput,
      offchainPublicKeys: validInput.offchainPublicKeys.map((key) => Buffer.from(key, 'hex')),
      reportingPluginConfig: reportingPluginConfigProto,
      sharedSecretEncryptions,
    }
    return {
      offchainConfig: Buffer.from(proto.encode('offchainreporting2_config.OffchainConfigProto', offchainConfig)),
      randomSecret,
    }
  }

  // constructs a SharedSecretEncryptions from
  // a set of SharedSecretEncryptionPublicKeys, the sharedSecret, and a cryptographic randomness source
  generateSecretEncryptions = async (
    operatorsPublicKeys: string[],
    secret?: string,
  ): Promise<{ sharedSecretEncryptions: sharedSecretEncryptions.SharedSecretEncryptions; randomSecret: string }> => {
    const gauntletSecret = process.env.SECRET
    const path = join(process.cwd(), 'packages/gauntlet-solana-contracts/artifacts/bip-0039', 'english.txt')
    const randomSecret = secret || (await sharedSecretEncryptions.generateSecretWords(path))
    return {
      sharedSecretEncryptions: sharedSecretEncryptions.makeSharedSecretEncryptions(
        gauntletSecret!,
        operatorsPublicKeys,
        randomSecret,
      ),
      randomSecret,
    }
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const input = this.makeInput(this.flags.input)
    const state = new PublicKey(this.flags.state)
    const userSecret = this.flags.secret
    this.require(userSecret, 'Please provide the secret flag with the secret generated at write time')

    logger.loading('Comparing pending onchain config with local config...')
    const onChainState = await program.account.state.fetch(state)
    const bufferedConfig = Buffer.from(onChainState.config.pendingOffchainConfig.xs).slice(
      0,
      new BN(onChainState.config.pendingOffchainConfig.len).toNumber(),
    )
    const { offchainConfig } = await this.serializeOffchainConfig(input, userSecret)

    this.require(
      Buffer.compare(bufferedConfig, offchainConfig) === 0,
      'Pending onchain config is different from the one generated',
    )
    logger.success('Onchain config matches with the local config')

    const data = program.coder.instruction.encode('commit_offchain_config', {})

    const accounts: AccountMeta[] = [
      {
        pubkey: state,
        isSigner: false,
        isWritable: true,
      },
      {
        pubkey: signer,
        isSigner: true,
        isWritable: false,
      },
    ]

    const rawTx: RawTransaction = {
      data,
      accounts,
      programId: ocr2.programId,
    }

    return [rawTx]
  }

  execute = async () => {
    const rawTx = await this.makeRawTransaction(this.wallet.publicKey)
    await prompt(`Commit Offchain config?`)
    const txhash = await this.signAndSendRawTx(rawTx)
    logger.success(`Committing offchain config on tx ${txhash}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(txhash, this.flags.state),
          contract: this.flags.state,
        },
      ],
    } as Result<TransactionResponse>
  }
}
