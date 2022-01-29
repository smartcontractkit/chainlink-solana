import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { RawTransaction, SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { AccountMeta, PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'

export default class ResetPendingOffchainConfig extends SolanaCommand {
  static id = 'ocr2:reset_pending_offchain_config'
  static category = CONTRACT_LIST.OCR_2

  static examples = ['yarn gauntlet ocr2:reset_pending_offchain_config --network=devnet --state=[OCR2_STATE]']
  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.state, 'Please provide flags with "state"')
  }

  makeRawTransaction = async (signer: PublicKey): Promise<RawTransaction[]> => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const state = new PublicKey(this.flags.state)

    const info = await program.account.state.fetch(state)
    console.log(info.config.pendingOffchainConfig)
    this.require(
      info.config.pendingOffchainConfig.version != 0 || info.config.pendingOffchainConfig.len != 0,
      'pending offchain config version is already in reset state',
    )

    const data = program.coder.instruction.encode('reset_pending_offchain_config', {})

    const accounts: AccountMeta[] = [
      {
        pubkey: state,
        isWritable: true,
        isSigner: false,
      },
      {
        pubkey: signer,
        isWritable: false,
        isSigner: true,
      },
    ]

    return [
      {
        data,
        accounts,
        programId: program.programId,
      },
    ]
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const rawTx = await this.makeRawTransaction(this.wallet.publicKey)
    await prompt(`Continue Reset pending offchain config?`)

    const txhash = await this.withIDL(this.signAndSendRawTx, program.idl)(rawTx)

    logger.success(`Reset pending offchain config on tx ${txhash}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(txhash, this.flags.state, { state: this.flags.state }),
          contract: this.flags.state,
        },
      ],
    } as Result<TransactionResponse>
  }
}
