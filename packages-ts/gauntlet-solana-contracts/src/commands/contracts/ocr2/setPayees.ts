import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { Token, TOKEN_PROGRAM_ID } from '@solana/spl-token'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../lib/contracts'

export default class SetPayees extends SolanaCommand {
  static id = 'ocr2:set_payees'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:set_payees --network=local --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC --keys=[OPERATORS]',
  ]

  constructor(flags, args) {
    super(flags, args)

    this.requireFlag('state', 'Provide a valid state address')
    this.requireFlag('keys', 'Provide payees info')
  }

  getPayableAddress = async (token, payee) => {
    return (await token.getOrCreateAssociatedAccountInfo(new PublicKey(payee))).address
  }

  execute = async () => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.publicKey.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const owner = this.wallet.payer
    const operators = this.flags.keys
    const state = new PublicKey(this.flags.state)

    const token = new Token(
      this.provider.connection,
      new PublicKey(this.flags.link),
      TOKEN_PROGRAM_ID,
      this.wallet.payer,
    )

    const info = await program.account.state.fetch(state)
    const payeeByTransmitter = ((await Promise.all(
      operators.map(async ({ NodeAddress, payeeAddress }) => ({
        transmitter: NodeAddress,
        // TODO: Will we receive an already payable address? If so, we don't need to get the associated account
        payee: await this.getPayableAddress(token, payeeAddress),
      })),
    )) as any[]).reduce(
      (agg, operator) => ({
        ...agg,
        [new PublicKey(operator.transmitter).toString()]: operator.payee,
      }),
      {},
    )
    // Set the payees in the same order the oracles are saved in the contract. The length of the payees need to be same as the oracles saved
    const payees = info.oracles
      .map(({ transmitter }) => payeeByTransmitter[new PublicKey(transmitter).toString()])
      .slice(0, info.config.n)

    logger.loading('Setting payees...')
    const tx = await program.rpc.setPayees(payees, {
      accounts: {
        state: state,
        authority: owner.publicKey,
      },
      signers: [owner],
    })

    return {
      responses: [
        {
          tx: this.wrapResponse(tx, state.toString()),
          contract: state.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
