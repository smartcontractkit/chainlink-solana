import { prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand } from '@chainlink/gauntlet-solana'
import { PublicKey, TransactionInstruction } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '../../lib/contracts'
import logger from '../../logger'

export default abstract class Close extends SolanaCommand {
  static makeId = (contractId: CONTRACT_LIST) => `${contractId}:close`
  static makeCategory = (contractId: CONTRACT_LIST) => contractId
  static makeDescription = (contractId: CONTRACT_LIST) =>
    `Closes a ${contractId} account returning the account funds to the signer. Only the contract owner can close it`
  static makeExamples = (contractId: CONTRACT_LIST) => [
    `yarn gauntlet ${contractId}:close --network=<NETWORK> <PROGRAM_STATE>`,
  ]

  contractId: CONTRACT_LIST

  constructor(flags, args) {
    super(flags, args)
    this.require(!!this.args[0], 'Please provide a valid account address as an argument')
  }

  prepareInstructions = async (
    signer: PublicKey,
    extraAccounts: { [key: string]: PublicKey } = {},
    closeFunction: string = 'close',
  ): Promise<TransactionInstruction[]> => {
    const contract = getContract(this.contractId, '')
    const address = contract.programId.toString()
    const program = this.loadProgram(contract.idl, address)

    const state = new PublicKey(this.args[0])

    logger.loading(
      `Preparing instruction to close account from ${this.contractId} contract with address ${logger.styleAddress(
        state.toString(),
      )}`,
    )

    const ix = program.instruction[closeFunction]({
      accounts: {
        receiver: signer,
        authority: signer,
        ...extraAccounts,
      },
    })

    return [ix]
  }

  execute = async () => {
    const state = new PublicKey(this.args[0])
    const ixs = await this.makeRawTransaction(this.wallet.publicKey)

    await prompt(`Continue closing ${this.contractId} state with address ${logger.styleAddress(state.toString())}?`)

    const tx = await this.signAndSendRawTx(ixs)

    logger.success(`Closed state ${logger.styleAddress(state.toString())} on tx ${tx}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(tx, state.toString(), { state: state.toString() }),
          contract: state.toString(),
        },
      ],
    }
  }
}
