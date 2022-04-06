import { join } from 'path'
import { contracts } from '@chainlink/gauntlet-solana'

export enum CONTRACT_LIST {
  ACCESS_CONTROLLER = 'access_controller',
  OCR_2 = 'ocr2',
  FLAGS = 'flags',
  STORE = 'store',
  TOKEN = 'token',
}

export const CONTRACT_ENV_NAMES = {
  [CONTRACT_LIST.ACCESS_CONTROLLER]: 'PROGRAM_ID_ACCESS_CONTROLLER',
  [CONTRACT_LIST.OCR_2]: 'PROGRAM_ID_OCR2',
  [CONTRACT_LIST.STORE]: 'PROGRAM_ID_STORE',
}

const SCHEMA_PATH = './artifacts/schemas'
const BINARY_PATH = './artifacts/bin'
const PROGRAM_ID_PATH = './artifacts/programId'

export const getContract = (name: CONTRACT_LIST, version: string) => {
  return contracts.getContract(name, version, CONTRACT_ENV_NAMES[name], SCHEMA_PATH)
}

export const getDeploymentContract = (name: CONTRACT_LIST, version: string) => {
  return contracts.getDeploymentContract(name, version, BINARY_PATH, PROGRAM_ID_PATH)
}
