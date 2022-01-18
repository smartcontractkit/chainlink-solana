import { Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse, RawTransaction } from '@chainlink/gauntlet-solana'
import { AccountMeta, PublicKey, Transaction, TransactionInstruction } from '@solana/web3.js'
import BN from 'bn.js'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'

export default class BeginOffchainConfig extends SolanaCommand {
  static id = 'ocr2:begin_offchain_config'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:begin_offchain_config --network=devnet --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
    'yarn gauntlet ocr2:begin_offchain_config --network=devnet --state=5oMNhuuRmxPGEk8ymvzJRAJFJGs7jaHsaxQ3Q2m6PVTR EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]
  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.state, 'Please provide flags with "state"')
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const state = new PublicKey(this.flags.state)
    const version = new BN(2)

    await prompt(`Begin setting Offchain config version ${version.toString()}?`)

    const data = program.coder.instruction.encode('begin_offchain_config', {
      offchainConfigVersion: version,
    })

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
    const rawTx = await this.makeRawTransaction(this.wallet.payer.publicKey)
    const tx = rawTx.reduce(
      (tx, meta) =>
        tx.add(
          new TransactionInstruction({
            programId: meta.programId,
            keys: meta.accounts,
            data: meta.data,
          }),
        ),
      new Transaction(),
    )
    logger.loading('Sending tx...')
    logger.debug(tx)
    const txhash = await this.provider.send(tx, [this.wallet.payer])
    logger.success(`Begin set offchain config on tx hash: ${txhash}`)

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

