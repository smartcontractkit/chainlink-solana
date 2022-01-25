import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse, RawTransaction } from '@chainlink/gauntlet-solana'
import { AccountMeta, PublicKey, SYSVAR_RENT_PUBKEY, SYSVAR_CLOCK_PUBKEY } from '@solana/web3.js'
import { UPGRADEABLE_BPF_LOADER_PROGRAM_ID } from '../../lib/constants'
import { CONTRACT_LIST, getContract } from '../../lib/contracts'
import { SolanaConstructor } from '../../lib/types'
import { encodeInstruction, makeTx } from '../../lib/utils'

export const makeUpgradeProgramCommand = (contractId: CONTRACT_LIST): SolanaConstructor => {
  return class UpgradeProgram extends SolanaCommand {
    static id = `${contractId}:upgrade_program`
    static category = contractId

    static examples = [`yarn gauntlet ${contractId}:upgrade_program --network=devnet --buffer=[BUFFER_ACCOUNT]`]

    constructor(flags, args) {
      super(flags, args)

      this.require(!!this.flags.buffer, 'Please provide flags with "buffer"')
    }

    makeRawTransaction = async (signer: PublicKey) => {
      const contract = getContract(contractId, '')

      const programId = new PublicKey(contract.programId)
      const [programDataKey, _nonce] = await PublicKey.findProgramAddress(
        [programId.toBuffer()],
        UPGRADEABLE_BPF_LOADER_PROGRAM_ID,
      )

      const buffer = new PublicKey(this.flags.buffer)
      const data = encodeInstruction({ Upgrade: {} })

      const accounts: AccountMeta[] = [
        { pubkey: programDataKey, isSigner: false, isWritable: true },
        { pubkey: programId, isSigner: false, isWritable: true },
        { pubkey: buffer, isSigner: false, isWritable: true },
        { pubkey: signer, isSigner: false, isWritable: true },
        { pubkey: SYSVAR_RENT_PUBKEY, isSigner: false, isWritable: false },
        { pubkey: SYSVAR_CLOCK_PUBKEY, isSigner: false, isWritable: false },
        { pubkey: signer, isSigner: true, isWritable: false },
      ]

      const rawTx: RawTransaction = {
        data,
        accounts,
        programId: UPGRADEABLE_BPF_LOADER_PROGRAM_ID,
      }

      return [rawTx]
    }

    execute = async () => {
      const rawTx = await this.makeRawTransaction(this.wallet.payer.publicKey)
      await prompt(`Continue upgrading the ${contractId} program?`)
      logger.loading('Upgrading program...')
      const txhash = await this.provider.send(makeTx(rawTx), [this.wallet.payer])
      logger.success(`Program upgraded on tx ${txhash}`)
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
}
