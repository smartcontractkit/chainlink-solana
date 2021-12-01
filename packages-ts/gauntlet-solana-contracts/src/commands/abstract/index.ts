import { ICommand, Result } from '@chainlink/gauntlet-core'
import { logger, prompt } from '@chainlink/gauntlet-core/dist/utils'
import { SolanaCommand, TransactionResponse } from '@chainlink/gauntlet-solana'
import { Contract, CONTRACT_LIST, getContract } from '../../lib/contracts'

enum SOLANA_OPERATIONS {
  DEPLOY = 'deploy',
  EXECUTE = 'execute',
  QUERY = 'query',
  HELP = 'help',
}
interface AbstractOpts {
  contract: Contract
  function: string
  action: SOLANA_OPERATIONS.DEPLOY | SOLANA_OPERATIONS.EXECUTE | SOLANA_OPERATIONS.QUERY | SOLANA_OPERATIONS.HELP
}

interface AbstractParams {
  args: any[]
  state: {
    accounts: any
    signers: any[]
    instructions: any[]
  }
}

export const makeAbstractCommand = async (
  instruction: string,
  flags: any,
  args: string[],
): Promise<ICommand | undefined> => {
  try {
    const commandOpts = await parseInstruction(instruction, flags.version)
    if (!commandOpts) return

    const params = parseParams(commandOpts, flags)
    if (!params) return

    return new GeneratorCommand(flags, args, commandOpts, params)
  } catch (e) {
    logger.error(e)
    return
  }
}

const parseInstruction = async (instruction: string, version: string): Promise<AbstractOpts | undefined> => {
  const isValidContract = (contractName: string): boolean => {
    // Validate that we have this contract available
    return Object.values(CONTRACT_LIST).includes(contractName as CONTRACT_LIST)
  }

  const isValidFunction = (abi, func): boolean => true
  const isQueryFunction = (abi, func): boolean => false

  const command = instruction.split(':')
  if (!command.length || command.length > 2) return

  const contract = isValidContract(command[0]) && (await getContract(command[0] as CONTRACT_LIST, version))
  if (!contract) throw new Error(`Abstract: Contract ${command[0]} not found`)

  if (command[1] === SOLANA_OPERATIONS.HELP) {
    return {
      contract,
      function: SOLANA_OPERATIONS.HELP,
      action: SOLANA_OPERATIONS.HELP,
    }
  }

  if (command[1] === SOLANA_OPERATIONS.DEPLOY) {
    return {
      contract,
      function: SOLANA_OPERATIONS.DEPLOY,
      action: SOLANA_OPERATIONS.DEPLOY,
    }
  }

  // TODO: Calls to contract functions not supported yet
  return

  // const functionName = isValidFunction(contract.abi, command[1]) && command[1]
  // if (!functionName) throw new Error(`Abstract: Function ${command[1]} for contract ${contract.id} not found`)

  // return {
  //   contract,
  //   function: functionName,
  //   action: isQueryFunction(contract.abi, functionName) ? SOLANA_OPERATIONS.QUERY : SOLANA_OPERATIONS.EXECUTE,
  // }
}

const parseParams = (commandOpts: AbstractOpts, params: any): AbstractParams | undefined => {
  return {
    args: [],
    state: {
      accounts: {},
      signers: [],
      instructions: [],
    },
  }
}

type AbstractExecute = (params: AbstractParams, address?: string) => Promise<Result<TransactionResponse>>

export default class GeneratorCommand extends SolanaCommand {
  opts: AbstractOpts
  params: AbstractParams

  // abstractDeploy: AbstractExecute
  abstractQuery: AbstractExecute
  // abstractExecute: AbstractExecute
  abstractHelp: AbstractExecute

  constructor(flags, args, opts, params) {
    super(flags, args)

    this.opts = opts
    this.params = params

    if ([...SOLANA_OPERATIONS.EXECUTE, ...SOLANA_OPERATIONS.QUERY].includes(this.opts.action)) {
      this.require(args[0], 'Provide a valid contract address')
    }
  }

  abstractDeploy: AbstractExecute = async () => {
    const balanceRequired = await this.provider.connection.getMinimumBalanceForRentExemption(
      this.opts.contract.bytecode.length,
    )
    const walletBalance = await this.provider.connection.getAccountInfo(this.wallet.publicKey)
    this.require(
      !!walletBalance && walletBalance?.lamports > balanceRequired,
      `Deployment cost is ${SolanaCommand.lamportsToSol(balanceRequired)}, higher than wallet balance: ${
        !!walletBalance && SolanaCommand.lamportsToSol(walletBalance.lamports)
      }`,
    )
    await prompt(`Deployment cost is ${SolanaCommand.lamportsToSol(balanceRequired)} SOL, continue?`)
    logger.loading(`Deploying ${this.opts.contract.id}...`)
    const tx = await this.deploy(this.opts.contract.bytecode, this.opts.contract.programId)
    const { success } = await tx.wait('')
    if (!success) {
      logger.error('Error deploying contract')
      return {} as Result<TransactionResponse>
    }
    logger.success(`Deployed ${this.opts.contract.id} to ${tx.address}`)
    return {
      responses: [
        {
          tx: tx,
          contract: tx.address,
        },
      ],
    } as Result<TransactionResponse>
  }

  abstractExecute: AbstractExecute = async (params, address) => {
    // Dt77xc3sAd5kwJiyWdJpbr4vxQNUwgsK9RmRZnqeGjwP
    console.log('SENDING TX')
    const tx = await this.executeProgram(address!, this.opts.contract.idl, this.opts.function, params.args)
    console.log('TX EXECUTED:', tx)
    return {
      responses: [
        {
          tx: {},
          contract: {},
        },
      ],
    } as Result<TransactionResponse>
  }

  execute = async () => {
    const operations = {
      [SOLANA_OPERATIONS.DEPLOY]: this.abstractDeploy,
      [SOLANA_OPERATIONS.QUERY]: this.abstractQuery,
      [SOLANA_OPERATIONS.EXECUTE]: this.abstractExecute,
      [SOLANA_OPERATIONS.HELP]: this.abstractHelp,
    }

    logger.log('Params: ', this.params)

    const address = this.args[0]
    return operations[this.opts.action](this.params, address)
  }
}

// IF SolanaCommand extends Generator (which extends WriteCommand), implementing the Generator interface, should give support for any chain interaction
interface Generator {
  execution: {
    deploy: AbstractExecute
    query: AbstractExecute
    write: AbstractExecute
    help: AbstractExecute
  }
  validation: {
    isValidFunction: () => {}
    isQueryFunction: () => {}
    parseParams: () => {}
  }
  contracts: {
    getContract: () => {}
  }
}
