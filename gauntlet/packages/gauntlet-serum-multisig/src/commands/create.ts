import { Result } from '@chainlink/gauntlet-core'
import { BN, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { PublicKey, SYSVAR_RENT_PUBKEY, Keypair } from '@solana/web3.js'
import { CONTRACT_LIST, getContract } from '@chainlink/gauntlet-solana-contracts'
import { withAddressBook } from '@chainlink/gauntlet-solana-contracts/dist/lib/middlewares'
import logger from '@chainlink/gauntlet-solana-contracts/dist/logger'

type Input = {
  owners: string[]
  threshold: number | string
}

export default class MultisigCreate extends SolanaCommand {
  static id = 'create'
  static category = CONTRACT_LIST.MULTISIG

  static examples = [
    'yarn gauntlet-serum-multisig create --network=local 3W37Aopzbtzczi8XWdkFTvBeSyYgXLuUkaodkq59xBCT ETqajtkz4xcsB397qTBPetprR8jMC3JszkjJJp3cjWJS QMaHW2Fpyet4ZVf7jgrGB6iirZLjwZUjN9vPKcpQrHs --threshold=2',
  ]

  constructor(flags, args) {
    super(flags, args)
    this.requireFlag('threshold', 'Please provide multisig threshold')
    this.use(withAddressBook)
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
    const maxOwners = this.flags.maxOwners || 30
    const owners = input.owners.map((key) => new PublicKey(key))

    // SIZE IN BYTES
    const OWNER_LENGTH = 32
    const EXTRA = 2
    const NONCE_LENGTH = 1
    const THRESHOLD_LENGTH = 8
    const SEQ_LENGTH = 4

    const TOTAL_TO_ALLOCATE = (OWNER_LENGTH + EXTRA) * maxOwners + THRESHOLD_LENGTH + NONCE_LENGTH + SEQ_LENGTH

    const threshold = new BN(input.threshold)
    await prompt(
      `A new multisig will be created with threshold ${threshold.toNumber()} and owners ${owners.map((o) =>
        logger.styleAddress(o.toString()),
      )}. Continue?`,
    )

    const instruction = await program.account.multisig.createInstruction(multisig, TOTAL_TO_ALLOCATE)
    const tx = await program.rpc.createMultisig(owners, new BN(input.threshold), nonce, {
      accounts: {
        multisig: multisig.publicKey,
        rent: SYSVAR_RENT_PUBKEY,
      },
      signers: [multisig],
      instructions: [instruction],
    })
    logger.success('New multisig created')
    logger.info(`Multisig address: ${multisig.publicKey}`)
    logger.info(`Multisig Signer: ${logger.styleAddress(multisigSigner.toString())}`)

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
