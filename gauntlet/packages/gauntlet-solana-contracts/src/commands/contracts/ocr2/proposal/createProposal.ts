import { Result } from '@chainlink/gauntlet-core'
import { logger, BN, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey, Keypair, SystemProgram } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'

export default class CreateProposal extends SolanaCommand {
  static id = 'ocr2:create_proposal'
  static category = CONTRACT_LIST.OCR_2

  static examples = ['yarn gauntlet ocr2:create_proposal --network=devnet']

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
    logger.log('Config Proposal state will be at:', proposal.toString())

    const createIx = await program.methods
      .createProposal(version)
      .accounts({
        proposal: proposal,
        authority: signer,
      })
      .instruction()
    const defaultAccountSize = new BN(program.account.proposal.size)
    const createAccountIx = SystemProgram.createAccount({
      fromPubkey: signer,
      newAccountPubkey: proposal,
      space: defaultAccountSize.toNumber(),
      lamports: await this.provider.connection.getMinimumBalanceForRentExemption(defaultAccountSize.toNumber()),
      programId: program.programId,
    })

    return [createAccountIx, createIx]
  }

  execute = async () => {
    const signer = this.wallet.publicKey

    const proposal = Keypair.generate()
    const rawTx = await this.makeRawTransaction(signer, proposal.publicKey)
    await this.simulateTx(signer, rawTx)
    await prompt(`Continue creating config proposal?`)

    const txhash = await this.signAndSendRawTx(rawTx, [proposal])
    logger.success(`Config Proposal created on tx ${txhash}`)
    logger.line()
    logger.info('Use the Config Proposal ID in future proposal commands:')
    logger.info(proposal.publicKey.toString())
    logger.line()

    return {
      responses: [
        {
          tx: this.wrapResponse(txhash, ''),
          contract: '',
        },
      ],
      data: {
        proposal: proposal.publicKey.toString(),
      },
    } as Result<TransactionResponse>
  }
}
