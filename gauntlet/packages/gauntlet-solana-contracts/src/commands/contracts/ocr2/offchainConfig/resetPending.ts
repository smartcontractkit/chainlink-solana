import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'

export default class ResetPendingOffchainConfig extends SolanaCommand {
  static id = 'ocr2:reset_pending_offchain_config'
  static category = CONTRACT_LIST.OCR_2

  static examples = ['yarn gauntlet ocr2:reset_pending_offchain_config --network=devnet --state=[OCR2_STATE]']
  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.state, 'Please provide flags with "state"')
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const state = new PublicKey(this.flags.state)
    const owner = this.wallet.payer

    const info = await program.account.state.fetch(state)
    console.log(info.config.pendingOffchainConfig)
    this.require(
      info.config.pendingOffchainConfig.version != 0 || info.config.pendingOffchainConfig.len != 0,
      'pending offchain config version is already in reset state',
    )

    await prompt(`Reset pending offchain config?`)

    const tx = await program.rpc.resetPendingOffchainConfig({
      accounts: {
        state: state,
        authority: owner.publicKey,
      },
    })

    logger.success(`Reset pending offchain config on tx ${tx}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(tx, state.toString(), { state: state.toString() }),
          contract: state.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
