import { io } from '@chainlink/gauntlet-core/dist/utils'
import { PublicKey } from '@solana/web3.js'
import { join } from 'path'

export type Contract = {
  id: CONTRACT_LIST
  version: string
  idl: any
  programId: PublicKey
}

export enum CONTRACT_LIST {
  ACCESS_CONTROLLER = 'access_controller',
  OCR_2 = 'ocr2',
  FLAGS = 'flags',
  STORE = 'store',
  TOKEN = 'token',
  MULTISIG = 'serum_multisig',
}

export const CONTRACT_ENV_NAMES = {
  [CONTRACT_LIST.ACCESS_CONTROLLER]: 'PROGRAM_ID_ACCESS_CONTROLLER',
  [CONTRACT_LIST.OCR_2]: 'PROGRAM_ID_OCR2',
  [CONTRACT_LIST.STORE]: 'PROGRAM_ID_STORE',
  [CONTRACT_LIST.MULTISIG]: 'PROGRAM_ID_MULTISIG',
}

export const getContract = (name: CONTRACT_LIST, version: string): Contract => ({
  id: name,
  version,
  idl: getContractSchema(name, version),
  programId: getProgramId(name),
})

const getContractSchema = (name: CONTRACT_LIST, version: string) => {
  return io.readJSON(join(process.cwd(), 'packages/gauntlet-solana-contracts/artifacts/schemas', `${name}`))
}

const getProgramId = (name: CONTRACT_LIST): PublicKey => {
  try {
    return new PublicKey(process.env[CONTRACT_ENV_NAMES[name]]!)
  } catch (e) {
    throw new Error(`No program id set for program ${name}. Set it in as env var with name ${CONTRACT_ENV_NAMES[name]}`)
  }
}
