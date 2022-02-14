import { Result } from '@chainlink/gauntlet-core'
import { logger, BN, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse, RawTransaction } from '@chainlink/gauntlet-solana'
import { AccountMeta, PublicKey, Keypair } from '@solana/web3.js'
import { Token, ASSOCIATED_TOKEN_PROGRAM_ID, TOKEN_PROGRAM_ID } from '@solana/spl-token'
import { utils } from '@project-serum/anchor'
import { ORACLES_MAX_LENGTH } from '../../../../lib/constants'
import { CONTRACT_LIST, getContract } from '../../../../lib/contracts'
import { getRDD } from '../../../../lib/rdd'
import { makeTx } from '../../../../lib/utils'

type Input = {
  digest: Buffer
}
export default class AcceptProposal extends SolanaCommand {
  static id = 'ocr2:accept_proposal'
  static category = CONTRACT_LIST.OCR_2

  static examples = [
    'yarn gauntlet ocr2:accept_proposal --network=devnet --state=EPRYwrb1Dwi8VT5SutS4vYNdF8HqvE7QwvqeCCwHdVLC',
  ]

  makeInput = (userInput): Input => {
    if (userInput) return userInput as Input
    // const rdd = getRDD(this.flags.rdd)
    // const aggregator = rdd.contracts[this.flags.state]
    // const aggregatorOperators: any[] = aggregator.oracles.map((o) => rdd.operators[o.operator])
    // const oracles = aggregatorOperators.map((operator) => ({
    //   // Same here
    //   transmitter: operator.ocrNodeAddress[0],
    //   signer: operator.ocr2OnchainPublicKey[0].replace('ocr2on_solana_', ''),
    // }))
    const digest = Buffer.alloc(32) // TODO

    return {
      digest,
    }
  }

  constructor(flags, args) {
    super(flags, args)

    this.require(!!this.flags.state, 'Please provide flags with "state"')
    this.require(!!this.flags.proposal, 'Please provide flags with "proposal"')
  }

  makeRawTransaction = async (signer: PublicKey) => {
    const ocr2 = getContract(CONTRACT_LIST.OCR_2, '')
    const address = ocr2.programId.toString()
    const program = this.loadProgram(ocr2.idl, address)

    const state = new PublicKey(this.flags.state)
    const proposal = new PublicKey(this.flags.proposal)
    const input = this.makeInput(this.flags.input)
    const link = new PublicKey(this.flags.link || process.env.LINK)

    logger.log('Config information:', input)

    // TODO: just fetch this from the state
    const [vaultAuthority] = await PublicKey.findProgramAddress(
      [Buffer.from(utils.bytes.utf8.encode('vault')), state.toBuffer()],
      program.programId,
    )

    const tokenVault = await Token.getAssociatedTokenAddress(
      ASSOCIATED_TOKEN_PROGRAM_ID,
      TOKEN_PROGRAM_ID,
      link,
      vaultAuthority,
      true,
    )

    // account = await program.account.state.fetch(state.publicKey);
    // let currentOracles = account.oracles.xs.slice(0, account.oracles.len);
    // let payees = currentOracles.map((oracle) => {
    //   return { pubkey: oracle.payee, isWritable: true, isSigner: false };
    // });

    let payees = [{ pubkey: Keypair.generate().publicKey, isWritable: true, isSigner: false }]

    // TODO: different receiver

    const tx = program.instruction.acceptProposal(input.digest, {
      accounts: {
        state: state,
        proposal: proposal,
        receiver: signer,
        authority: signer,
        tokenVault: tokenVault,
        vaultAuthority: vaultAuthority,
        tokenProgram: TOKEN_PROGRAM_ID,
      },
      remainingAccounts: payees,
    })

    const rawTx: RawTransaction = {
      data: tx.data,
      accounts: tx.keys,
      programId: tx.programId,
    }

    return [rawTx]
  }

  execute = async () => {
    const rawTx = await this.makeRawTransaction(this.wallet.publicKey)
    await prompt(`Continue setting config on ${this.flags.state.toString()}?`)
    const txhash = await this.signAndSendRawTx(rawTx)
    logger.success(`Accepted proposal on tx ${txhash}`)

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
