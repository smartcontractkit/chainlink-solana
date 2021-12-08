import { io } from '@chainlink/gauntlet-core/dist/utils'
import { Keypair } from '@solana/web3.js'
import { readFileSync } from 'fs'
import { join } from 'path'

export type Contract = {
  id: CONTRACT_LIST
  bytecode: Buffer
  version: string
  idl: any
  programId: Keypair
}

export enum CONTRACT_LIST {
  ACCESS_CONTROLLER = 'access_controller',
  OCR_2 = 'ocr2',
  FLAGS = 'flags',
  DEVIATION_FLAGGING_VALIDATOR = 'deviation_flagging_validator',
  TOKEN = 'token',
}

export const getContract = (name: CONTRACT_LIST, version: string): Contract => ({
  id: name,
  bytecode: getContractCode(name, version),
  version,
  idl: getContractSchema(name, version),
  programId: getProgramId(name, version),
})

// TODO: Get it from GH Releases
const getContractCode = (name: CONTRACT_LIST, version: string) => {
  return readFileSync(join(process.cwd(), 'packages/gauntlet-solana-contracts/artifacts/bin', `${name}.so`))
}

const getContractSchema = (name: CONTRACT_LIST, version: string) => {
  return io.readJSON(join(process.cwd(), 'packages/gauntlet-solana-contracts/artifacts/schemas', `${name}`))
}

const getProgramId = (name: CONTRACT_LIST, version: string) => {
  const rawPK = io.readJSON(join(process.cwd(), 'packages/gauntlet-solana-contracts/artifacts/programId', `${name}`))
  return Keypair.fromSecretKey(Uint8Array.from(rawPK))
}
