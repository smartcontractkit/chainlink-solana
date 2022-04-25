import { Connection, ParsedTransactionWithMeta, PublicKey } from '@solana/web3.js'
import { EventParser, Event, Program } from '@project-serum/anchor'

/*
  Gets the latest transactions to a contract
  @param account address of contract
  @param limit number of transactions to return
  @param before transaction signature to start at
  */
const getLatestTxns = async (
  connection: Connection,
  account: PublicKey,
  limit: number,
  before?: string,
): Promise<(null | ParsedTransactionWithMeta)[]> => {
  // Get latest sigs
  const sigs = await connection.getSignaturesForAddress(account, {
    limit,
    before,
  })
  // Get the txns associated with the sigs
  const txns = await connection.getParsedTransactions(sigs.map((sig) => sig.signature))
  return txns
}

const parseTxLog = (parser: EventParser, tx: ParsedTransactionWithMeta | null): Event[] => {
  const eventList: Event[] = []
  const addToList = (event: Event) => eventList.push(event)

  if (tx?.meta?.logMessages) {
    parser.parseLogs(tx.meta.logMessages, addToList)
  }

  return eventList
}

// TODO: Add some inline doc
export const getLatestEvents = async (
  connection: Connection,
  account: PublicKey,
  program: Program,
  eventName: string,
  lastSigChecked?: string,
  rounds = 10,
  batch = 10,
): Promise<Event[]> => {
  if (rounds === 0) return []

  const latestTxs = await getLatestTxns(connection, account, batch, lastSigChecked)
  const eventParser = new EventParser(program.programId, program.coder)

  const newTransmissionLogs = latestTxs
    .map((tx) => parseTxLog(eventParser, tx))
    .reduce((agg, txEvents) => [...agg, ...txEvents], [])
    .filter((event) => event.name === eventName)

  if (newTransmissionLogs.length > 0) return newTransmissionLogs
  return [
    ...newTransmissionLogs,
    ...(await getLatestEvents(
      connection,
      account,
      program,
      eventName,
      latestTxs[latestTxs.length - 1]?.transaction.signatures[0],
      rounds - 1,
      batch,
    )),
  ]
}
