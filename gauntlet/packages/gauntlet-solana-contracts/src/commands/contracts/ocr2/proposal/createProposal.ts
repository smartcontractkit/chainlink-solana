import { Result } from '@chainlink/gauntlet-core'
import { logger, BN, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse, RawTransaction } from '@chainlink/gauntlet-solana'
import { PublicKey, Keypair } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'

export default class CreateProposal extends SolanaCommand {
  static id = 'ocr2:create_proposal'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:create_proposal --network=devnet --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]

  constructor(flags, args) {
    super(flags, args)
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const version = new BN(2)

    const proposal = Keypair.generate()

    logger.log('Generating data for creating config proposal')
    logger.log('Proposal state will be at:', proposal.publicKey.toString())

    const createIx = await program.instruction.createProposal(version, {
      accounts: {
        proposal: proposal.publicKey,
        authority: signer,
      },
    })
    const createAccountIx = await program.account.proposal.createInstruction(proposal)

    const rawTxs: RawTransaction[] = [
      {
        data: createAccountIx.data,
        accounts: createAccountIx.keys,
        programId: createAccountIx.programId,
      },
      {
        data: createIx.data,
        accounts: createIx.keys,
        programId: createIx.programId,
      },
    ]

    return rawTxs
  }

  execute = async () => {
    const rawTx = await this.makeRawTransaction(this.wallet.publicKey)
    await prompt(`Continue creating config proposal?`)
    const txhash = await this.signAndSendRawTx(rawTx)
    logger.success(`Proposal created on tx ${txhash}`)

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
