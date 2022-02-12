import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt, BN } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse, RawTransaction } from '@chainlink/gauntlet-solana'
import { AccountMeta, PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'
import WriteOffchainConfig, { Input } from './write'
import { serializeOffchainConfig } from '../../../../lib/encoding'
import RDD from '../../../../lib/rdd'

export default class CommitOffchainConfig extends SolanaCommand {
  static id = 'ocr2:commit_offchain_config'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:commit_offchain_config --network=devnet --rdd=[PATH_TO_RDD] EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
    'yarn gauntlet ocr2:commit_offchain_config EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]
  constructor(flags, args) {
    super(flags, args)

    this.require(
      !!process.env.SECRET,
      'Please specify the Gauntlet secret words e.g. SECRET="awe fluke polygon tonic lilly acuity onyx debra bound gilbert wane"',
    )
  }

  makeInput = (userInput: any): Input => {
    if (userInput) return userInput as Input
    const network = this.flags.network || ''
    const rddPath = this.flags.rdd || ''
    const rdd = RDD.load(network, rddPath)
    return WriteOffchainConfig.makeInputFromRDD(rdd, this.args[0])
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const input = this.makeInput(this.flags.input)
    const state = new PublicKey(this.args[0])
    const userSecret = this.flags.secret
    this.require(userSecret, 'Please provide the secret flag with the secret generated at write time')

    logger.loading('Comparing pending onchain config with local config...')
    const onChainState = await program.account.state.fetch(state)
    const bufferedConfig = Buffer.from(onChainState.config.pendingOffchainConfig.xs).slice(
      0,
      new BN(onChainState.config.pendingOffchainConfig.len).toNumber(),
    )

    // process.env.SECRET is required on the command
    const gauntletSecret = process.env.SECRET!
    const { offchainConfig } = await serializeOffchainConfig(input, gauntletSecret, userSecret)

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
