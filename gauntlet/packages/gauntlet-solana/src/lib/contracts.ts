import { readFileSync } from 'fs'
import { join } from 'path'
import { io } from '@chainlink/gauntlet-core/dist/utils'
import { Keypair, PublicKey } from '@solana/web3.js'

export type Contract = {
  id: string
  version: string
  idl: any
  programId: PublicKey
}

export type DeploymentContract = {
  id: string
  bytecode: Buffer
  programKeypair: Keypair
}

export const registerContracts = <List extends Record<string, string>>(
  list: List,
  listProgramIdEnvNames: Record<List[keyof List], string>,
  artifactsPath: string,
) => {
  type ListValue = List[keyof List]
  // TODO: Get it from GH Releases
  const _getContractCode = (name: ListValue, version: string) => {
    try {
      return readFileSync(join(`${artifactsPath}/bin`, `${name}.so`))
    } catch (e) {
      throw new Error(`No program binary found for ${name} contract`)
    }
  }

  const _getContractSchema = (name: ListValue, version: string) => {
    return io.readJSON(join(`${artifactsPath}/schemas`, `${name}`))
  }

  const _getProgramKeypair = (name: ListValue, version: string): Keypair => {
    try {
      const rawPK = io.readJSON(join(`${artifactsPath}/programId`, `${name}`))
      return Keypair.fromSecretKey(Uint8Array.from(rawPK))
    } catch (e) {
      throw new Error(`No program id keypair set for program ${name}`)
    }
  }

  const _getProgramId = (name: ListValue): PublicKey => {
    try {
      const envName = listProgramIdEnvNames[name]
      return new PublicKey(process.env[envName])
    } catch (e) {
      throw new Error(`No program id found set for program ${name}`)
    }
  }

  const getContract = (name: ListValue, version?: string): Contract => ({
    id: name,
    version,
    idl: _getContractSchema(name, version),
    programId: _getProgramId(name),
  })

  const getDeploymentContract = (name: ListValue, version?: string): DeploymentContract => ({
    id: name,
    programKeypair: _getProgramKeypair(name, version),
    bytecode: _getContractCode(name, version),
  })

  return {
    getContract,
    getDeploymentContract,
  }
}
