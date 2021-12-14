import { join } from 'path'
import Proto from 'protobufjs'
export class Protobuf {
  root: Proto.Root

  static makeRootFromProto = async (protoName: string): Promise<Proto.Root> => {
    return await new Promise((res, rej) => {
      const path = join(process.cwd(), 'packages/gauntlet-solana-contracts/src/core/proto', protoName)
      Proto.load(path, (err, root) => {
        if (err) rej(err)
        res(root!)
      })
    })
  }

  constructor({ descriptor, root }: { descriptor?: any; root?: Proto.Root }) {
    if (root) {
      this.root = root
      return
    }
    this.root = Proto.Root.fromJSON(descriptor)
  }

  encode = (type: string, payload: any): Uint8Array => {
    const protoType = this.root.lookupType(type)
    let err = protoType.verify(payload)
    if (err) {
      throw new Error(`Protobuf: Payload does not match descriptor ${type}`)
    }

    return protoType.encode(protoType.create(payload)).finish()
  }

  // Throws if invalid buffer for this type
  decode = (type: string, buffer: Buffer | Uint8Array): any => {
    const protoType = this.root.lookupType(type)
    return protoType.decode(buffer)
  }

  toJSON = () => {
    return this.root.toJSON()
  }
}
