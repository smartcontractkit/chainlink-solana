import { join } from 'path'
import Proto, { util, Long } from 'protobufjs'

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

  falsyToNullProperties = (payload: any): any => {
    const copy = { ...payload }
    Object.keys(copy).forEach((key) => {
      if (typeof copy[key] === 'object') {
        this.falsyToNullProperties(copy[key])
      } else {
        const noFalsyValue = copy[key] || null
        copy[key] = noFalsyValue
      }
    })
    return copy
  }

  encode = (type: string, payload: any): Uint8Array => {
    const protoType = this.root.lookupType(type)
    // Proto encoding should ignore falsy values. In protobuf.js we need to assign null to falsy to be ignored
    const noFalsyPayload = this.falsyToNullProperties(payload)
    let err = protoType.verify(noFalsyPayload)
    if (err) {
      throw new Error(`Protobuf: Payload does not match descriptor ${type}`)
    }

    return protoType.encode(protoType.create(noFalsyPayload)).finish()
  }

  // Throws if invalid buffer for this type
  decode = (type: string, buffer: Buffer | Uint8Array): any => {
    const protoType = this.root.lookupType(type)
    return protoType.decode(buffer)
  }

  static longToString = (n: Long): string => {
    return new util.Long(n.low, n.high, n.unsigned).toString()
  }

  toJSON = () => {
    return this.root.toJSON()
  }
}
