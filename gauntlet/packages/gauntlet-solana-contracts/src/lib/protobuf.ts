import Proto from 'protobufjs'

export class Protobuf {
  root: Proto.Root

  constructor(descriptor: any) {
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
}
