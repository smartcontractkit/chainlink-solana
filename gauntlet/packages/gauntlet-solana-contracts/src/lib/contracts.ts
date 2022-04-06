import { contracts, utils } from '@chainlink/gauntlet-solana'

export enum CONTRACT_LIST {
  ACCESS_CONTROLLER = 'access_controller',
  OCR_2 = 'ocr2',
  STORE = 'store',
  TOKEN = 'token',
}

export const CONTRACT_ENV_NAMES = {
  [CONTRACT_LIST.ACCESS_CONTROLLER]: 'PROGRAM_ID_ACCESS_CONTROLLER',
  [CONTRACT_LIST.OCR_2]: 'PROGRAM_ID_OCR2',
  [CONTRACT_LIST.STORE]: 'PROGRAM_ID_STORE',
  [CONTRACT_LIST.TOKEN]: 'PROGRAM_ID_TOKEN',
}

export const { getContract, getDeploymentContract } = contracts.registerContracts(
  CONTRACT_LIST,
  CONTRACT_ENV_NAMES,
  'packages/gauntlet-solana-contracts/artifacts',
)
