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

export const getContract = (name: string, version: string, envVar: string, path: string): Contract => ({
    id: name,
    version,
    idl: getContractSchema(name, version, path),
    programId: getProgramId(name, envVar),
  })
  
  export const getDeploymentContract = (name: string, version: string, binaryPath: string, programIdPath: string): DeploymentContract => ({
    id: name,
    programKeypair: getProgramKeypair(name, version, programIdPath),
    bytecode: getContractCode(name, version, binaryPath),
  })
  
  // TODO: Get it from GH Releases
  const getContractCode = (name: string, version: string, path: string) => {
    try {
      return readFileSync(join(path, `${name}.so`))
    } catch (e) {
      throw new Error(`No program binary found for ${name} contract`)
    }
  }
  
  const getContractSchema = (name: string, version: string, path: string) => {
    return io.readJSON(join(path, `${name}`))
  }
  
  const getProgramKeypair = (name: string, version: string, path: string): Keypair => {
    try {
      const rawPK = io.readJSON(join(path, `${name}`))
      return Keypair.fromSecretKey(Uint8Array.from(rawPK))
    } catch (e) {
      throw new Error(`No program id keypair set for program ${name}`)
    }
  }
  
  const getProgramId = (name: string, envVar: string): PublicKey => {
    try {
      return new PublicKey(process.env[envVar]!)
    } catch (e) {
      throw new Error(`No program id set for program ${name}. Set it in as env var with name ${envVar}`)
    }
  }
  