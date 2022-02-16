import { Result } from '@chainlink/gauntlet-core'
import { logger, BN, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse, RawTransaction } from '@chainlink/gauntlet-solana'
import { PublicKey, Keypair, SystemProgram } from '@solana/web3.js'
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

  makeRawTransaction = async (signer: PublicKey, proposal?: PublicKey) => {
    if (!proposal) throw new Error('Proposal address is required')

    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const version = new BN(2)

    logger.log('Generating data for creating config proposal')
    logger.log('Proposal state will be at:', proposal.toString())

    const createIx = await program.instruction.createProposal(version, {
      accounts: {
        proposal: proposal,
        authority: signer,
      },
    })
    const defaultAccountSize = new BN(program.account.proposal.size)
    const createAccountIx = await SystemProgram.createAccount({
      fromPubkey: signer,
      newAccountPubkey: proposal,
      space: defaultAccountSize.toNumber(),
      lamports: await this.provider.connection.getMinimumBalanceForRentExemption(defaultAccountSize.toNumber()),
      programId: program.programId,
    })

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
    const proposal = Keypair.generate()
    const rawTx = await this.makeRawTransaction(this.wallet.publicKey, proposal.publicKey)
    await prompt(`Continue creating config proposal?`)
    const txhash = await this.signAndSendRawTx(rawTx, [proposal])
    logger.success(`Proposal created on tx ${txhash}`)
    logger.line()
    logger.info('Use the proposal ID in future proposal commands:')
    logger.info(proposal.publicKey.toString())
    logger.line()

    return {
      responses: [
        {
          tx: this.wrapResponse(txhash, this.flags.state),
          contract: this.flags.state,
        },
      ],
      data: {
        proposal: proposal.publicKey.toString(),
      },
    } as Result<TransactionResponse>
  }
}
