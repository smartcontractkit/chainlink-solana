import * as anchor from "@project-serum/anchor";
import { BN } from "@project-serum/anchor";
import { Connection, PublicKey } from "@solana/web3.js";

export const CHAINLINK_AGGREGATOR_PROGRAM_ID = new PublicKey("HW3ipKzeeduJq6f1NqRCw4doknMeWkfrM4WxobtG3o5v");
export const CHAINLINK_STORE_PROGRAM_ID = new PublicKey("CaH12fwNTKJAG8PxEvo9R96Zc2j8qNHZaFj8ZW49yZNT");

export interface Round {
  contract: PublicKey
  answer: BN
  roundId: number
  epoch: number
  aggregatorRoundId: number
  observationsTS: Date
}

export class OCR2Feed {
  private _parser: anchor.EventParser;

  constructor(
    readonly aggregatorProgram: anchor.Program,
    readonly provider: anchor.Provider,
  ) {
    this._parser = new anchor.EventParser(aggregatorProgram.programId, aggregatorProgram.coder);
  }

  static async load(
    programID: PublicKey = CHAINLINK_AGGREGATOR_PROGRAM_ID,
    provider?: anchor.Provider,
  ): Promise<OCR2Feed> {
    provider = provider ?? anchor.getProvider();
    const aggregatorProgram = await anchor.Program.at(programID, provider);
    return new OCR2Feed(aggregatorProgram, provider);
  }

  public onRound(feed: PublicKey, callback: (round: Round) => void): number {
    return this.provider.connection.onLogs(feed, (event, ctx) => {
        this._parser.parseLogs(event.logs, (log) => {
          if (log.name != "NewTransmission") {
            return;
          }
          let answer = log.data.answer as Uint8Array;
          let event = {
            ...log.data,
            answer: new BN(answer, 10, 'le'),
            slot: ctx.slot, //TODO ?
            contract: feed,
            roundId: -1, //TODO
            epoch: -1, //TODO
            aggregatorRoundId: -1, //TODO
            observationsTS: new Date(),
          };
          callback(event);
        })
    })
  }

  public async removeListener(listener: number): Promise<void> {
    return this.provider.connection.removeOnLogsListener(listener)
  }
}
