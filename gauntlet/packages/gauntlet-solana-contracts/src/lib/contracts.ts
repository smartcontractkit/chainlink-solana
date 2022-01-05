import { io, logger } from '@chainlink/gauntlet-core/dist/utils'
import { Keypair, PublicKey } from '@solana/web3.js'
import { readFileSync } from 'fs'
import { join } from 'path'

export type Contract = {
  id: CONTRACT_LIST
  version: string
  idl: any
  programId: PublicKey
}

export type DeploymentContract = {
  id: CONTRACT_LIST
  bytecode: Buffer
  programKeypair: Keypair
}

export enum CONTRACT_LIST {
  ACCESS_CONTROLLER = 'access_controller',
  OCR_2 = 'ocr2',
  FLAGS = 'flags',
  DEVIATION_FLAGGING_VALIDATOR = 'deviation_flagging_validator',
  TOKEN = 'token',
  MULTISIG = 'serum_multisig',
}

export const CONTRACT_ENV_NAMES = {
  [CONTRACT_LIST.ACCESS_CONTROLLER]: 'PROGRAM_ID_ACCESS_CONTROLLER',
  [CONTRACT_LIST.OCR_2]: 'PROGRAM_ID_OCR2',
  [CONTRACT_LIST.DEVIATION_FLAGGING_VALIDATOR]: 'PROGRAM_ID_DEVIATION_FLAGGING_VALIDATOR',
  [CONTRACT_LIST.MULTISIG]: 'PROGRAM_ID_MULTISIG',

}

export const getContract = (name: CONTRACT_LIST, version: string): Contract => ({
  id: name,
  version,
  idl: getContractSchema(name, version),
  programId: getProgramId(name),
})

export const getDeploymentContract = (name: CONTRACT_LIST, version: string): DeploymentContract => ({
  id: name,
  programKeypair: getProgramKeypair(name, version),
  bytecode: getContractCode(name, version),
})

// TODO: Get it from GH Releases
const getContractCode = (name: CONTRACT_LIST, version: string) => {
  try {
    return readFileSync(join(process.cwd(), 'packages/gauntlet-solana-contracts/artifacts/bin', `${name}.so`))
  } catch (e) {
    throw new Error(`No program binary found for ${name} contract`)
  }
}

const getContractSchema = (name: CONTRACT_LIST, version: string) => {
  return io.readJSON(join(process.cwd(), 'packages/gauntlet-solana-contracts/artifacts/schemas', `${name}`))
}

const getProgramKeypair = (name: CONTRACT_LIST, version: string): Keypair => {
  try {
    const rawPK = io.readJSON(join(process.cwd(), 'packages/gauntlet-solana-contracts/artifacts/programId', `${name}`))
    return Keypair.fromSecretKey(Uint8Array.from(rawPK))
  } catch (e) {
    throw new Error(`No program id keypair set for program ${name}`)
  }
}

const getProgramId = (name: CONTRACT_LIST): PublicKey => {
  try {
    return new PublicKey(process.env[CONTRACT_ENV_NAMES[name]]!)
  } catch (e) {
    throw new Error(`No program id set for program ${name}. Set it in as env var with name ${CONTRACT_ENV_NAMES[name]}`)
  }
}
