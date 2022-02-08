import { Proto, sharedSecretEncryptions } from '@chainlink/gauntlet-core/dist/crypto'
import { join } from 'path'
import { Input as OffchainConfigInput } from '../commands/contracts/ocr2/offchainConfig/write'
import { descriptor as OCR2Descriptor } from './ocr2Proto'

export const serializeOffchainConfig = async (
  input: OffchainConfigInput,
  gauntletSecret: string,
  secret?: string,
): Promise<{ offchainConfig: Buffer; randomSecret: string }> => {
  const { configPublicKeys, ...validInput } = input
  const proto = new Proto.Protobuf({ descriptor: OCR2Descriptor })
  const reportingPluginConfigProto = proto.encode(
    'offchainreporting2_config.ReportingPluginConfig',
    validInput.reportingPluginConfig,
  )
  const { sharedSecretEncryptions, randomSecret } = await generateSecretEncryptions(
    configPublicKeys,
    gauntletSecret,
    secret,
  )
  const offchainConfig = {
    ...validInput,
    offchainPublicKeys: validInput.offchainPublicKeys.map((key) => Buffer.from(key, 'hex')),
    reportingPluginConfig: reportingPluginConfigProto,
    sharedSecretEncryptions,
  }
  return {
    offchainConfig: Buffer.from(proto.encode('offchainreporting2_config.OffchainConfigProto', offchainConfig)),
    randomSecret,
  }
}

// constructs a SharedSecretEncryptions from
// a set of SharedSecretEncryptionPublicKeys, the sharedSecret, and a cryptographic randomness source
const generateSecretEncryptions = async (
  operatorsPublicKeys: string[],
  gauntletSecret: string,
  secret?: string,
): Promise<{ sharedSecretEncryptions: sharedSecretEncryptions.SharedSecretEncryptions; randomSecret: string }> => {
  const path = join(process.cwd(), 'packages/gauntlet-solana-contracts/artifacts/bip-0039', 'english.txt')
  const randomSecret = secret || (await sharedSecretEncryptions.generateSecretWords(path))
  return {
    sharedSecretEncryptions: sharedSecretEncryptions.makeSharedSecretEncryptions(
      gauntletSecret,
      operatorsPublicKeys,
      randomSecret,
    ),
    randomSecret,
  }
}
