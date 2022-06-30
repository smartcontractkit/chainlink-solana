import { Result } from '@chainlink/gauntlet-core'
import { prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse, logger } from '@chainlink/gauntlet-solana'
import { PublicKey } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'

export default class FinalizeProposal extends SolanaCommand {
  static id = 'ocr2:finalize_proposal'
  static category = CONTRACT_LIST.OCR_2

  static examples = ['yarn gauntlet ocr2:finalize_proposal --network=devnet --proposalId=<PROPOSAL_ID>']

  constructor(flags, args) {
    super(flags, args)
    this.require(
      !!this.flags.proposalId || !!this.flags.configProposal,
      'Please provide Config Proposal ID with flag "proposalId" or "configProposal"',
    )
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)
    const proposalId = this.flags.proposalId || this.flags.configProposal

    const proposal = new PublicKey(proposalId)
    const finalizeIx = program.instruction.finalizeProposal({
      accounts: {
        proposal: proposal,
        authority: signer,
      },
    })

    return [finalizeIx]
  }

  execute = async () => {
    const signer = this.wallet.publicKey

    const rawTx = await this.makeRawTransaction(signer)
    await this.simulateTx(signer, rawTx)
    await prompt(`Continue finalizing Config Proposal?`)

    const txhash = await this.signAndSendRawTx(rawTx)
    logger.success(`Config Proposal finalized on tx ${txhash}`)

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
