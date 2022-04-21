import { Connection, PublicKey } from '@solana/web3.js'
import { Program } from '@project-serum/anchor'
import { provider } from '@chainlink/gauntlet-solana'
import { BN } from '@chainlink/gauntlet-core/dist/utils'

type NewTransmission = {
  roundId: number
  configDigest: Buffer
  answer: BN
  transmitter: number
  observationsTimestamp: number
  observerCount: number
  observers: string[]
  juelsPerLamport: BN
  reimbursementGjuels: BN
}

const parseNewTransmissionEvent = (event: any): NewTransmission => {
  return {
    roundId: Number(event.data.roundId),
    configDigest: Buffer.from(event.data.configDigest),
    answer: new BN(event.data.answer),
    transmitter: Number(event.data.transmitter),
    observationsTimestamp: event.data.observationsTimestamp,
    observerCount: Number(event.data.observerCount),
    observers: event.data.observers,
    juelsPerLamport: new BN(event.data.juelsPerLamport),
    reimbursementGjuels: new BN(event.data.reimbursementGjuels),
  }
}

export const getLatestNewTransmissionEvents = async (
  connection: Connection,
  state: PublicKey,
  program: Program,
): Promise<NewTransmission[]> => {
  const events = await provider.getLatestEvents(connection, state, program, 'NewTransmission')
  return events.map(parseNewTransmissionEvent)
}
