import { Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey, SYSVAR_RENT_PUBKEY, Keypair } from '@solana/web3.js'
import BN from 'bn.js'
import { CONTRACT_LIST, getContract } from '@chainlink/gauntlet-solana-contracts'

type Input = {
  owners: string[]
  threshold: number | string
}

const DEFAULT_MAXIMUM_SIZE = 200

export default class MultisigCreate extends SolanaCommand {
  static id = 'create'
  static category = CONTRACT_LIST.MULTISIG

  static examples = ['yarn gauntlet-serum-multisig create --network=local']

  constructor(flags, args) {
    super(flags, args)
    this.requireFlag('threshold', 'Please provide multisig threshold')
  }

  makeInput = (userInput: any): Input => {
    if (userInput) return userInput as Input

    return {
      //TODO: validate args. Maybe wrap them int PublicKey?
      owners: this.args,
      threshold: this.flags.threshold,
    }
  }

  execute = async () => {
    this.require(this.args.length > 0, 'Please provide at least one owner as an argument')
    const contract = getContract(CONTRACT_LIST.MULTISIG, '')
    const address = contract.programId.toString()
    const program = this.loadProgram(contract.idl, address)

    const input = this.makeInput(this.flags.input)

    const multisig = Keypair.generate()

    const [multisigSigner, nonce] = await PublicKey.findProgramAddress(
      [multisig.publicKey.toBuffer()],
      program.programId,
    )
    const maximumSize = this.flags.maximumSize || DEFAULT_MAXIMUM_SIZE
    const owners = input.owners.map((key) => new PublicKey(key))

    const tx = await program.rpc.createMultisig(owners, new BN(input.threshold), nonce, {
      accounts: {
        multisig: multisig.publicKey,
        rent: SYSVAR_RENT_PUBKEY,
      },
      signers: [multisig],
      instructions: [await program.account.multisig.createInstruction(multisig, maximumSize)],
    })
    logger.info(`Multisig address: ${multisig.publicKey}`)
    logger.info(`Multisig Signer: ${multisigSigner.toString()}`)

    return {
      responses: [
        {
          tx: this.wrapResponse(tx, multisig.publicKey.toString()),
          contract: multisig.publicKey.toString(),
        },
      ],
    } as Result<TransactionResponse>
  }
}
