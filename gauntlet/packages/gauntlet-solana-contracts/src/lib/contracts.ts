import { io, logger } from '@chainlink/gauntlet-core/dist/utils'
import { Keypair, PublicKey } from '@solana/web3.js'
import { readFileSync } from 'fs'
import { join } from 'path'

export type Contract = {
  id: CONTRACT_LIST
  bytecode: Buffer
  version: string
  idl: any
  programId: PublicKey | undefined
  programKeypair: Keypair | undefined
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
  programId: getProgramId(name),
  programKeypair: getProgramKeypair(name, version),
})

// TODO: Get it from GH Releases
const getContractCode = (name: CONTRACT_LIST, version: string) => {
  return readFileSync(join(process.cwd(), 'packages/gauntlet-solana-contracts/artifacts/bin', `${name}.so`))
}

const getContractSchema = (name: CONTRACT_LIST, version: string) => {
  return io.readJSON(join(process.cwd(), 'packages/gauntlet-solana-contracts/artifacts/schemas', `${name}`))
}

const getProgramKeypair = (name: CONTRACT_LIST, version: string): Keypair | undefined => {
  try {
    const rawPK = io.readJSON(join(process.cwd(), 'packages/gauntlet-solana-contracts/artifacts/programId', `${name}`))
    return Keypair.fromSecretKey(Uint8Array.from(rawPK))
  } catch (e) {
    logger.warn(`No program id keypair set for program ${name}`)
    return
  }
}

const getProgramId = (name: CONTRACT_LIST): PublicKey | undefined => {
  const envNames = {
    [CONTRACT_LIST.ACCESS_CONTROLLER]: 'ACCESS_CONTROLLER',
    [CONTRACT_LIST.OCR_2]: 'OCR2',
    [CONTRACT_LIST.DEVIATION_FLAGGING_VALIDATOR]: 'DEVIATION_FLAGGING_VALIDATOR',
  }

  try {
    return new PublicKey(process.env[envNames[name]]!)
  } catch (e) {
    logger.warn(`No program id set for program ${name}`)
    return
  }
}
