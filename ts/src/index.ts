import * as anchor from "@project-serum/anchor";
import { BN } from "@project-serum/anchor";
import { PublicKey } from "@solana/web3.js";

export const CHAINLINK_AGGREGATOR_PROGRAM_ID = new PublicKey(
  "HW3ipKzeeduJq6f1NqRCw4doknMeWkfrM4WxobtG3o5v"
);
export const CHAINLINK_STORE_PROGRAM_ID = new PublicKey(
  "CaH12fwNTKJAG8PxEvo9R96Zc2j8qNHZaFj8ZW49yZNT"
);

export interface Round {
  feed: PublicKey;
  answer: BN;
  roundId: number;
  observationsTS: Date;
}

export class OCR2Feed {
  private _parser: anchor.EventParser;

  constructor(
    readonly aggregatorProgram: anchor.Program,
    readonly provider: anchor.Provider
  ) {
    this._parser = new anchor.EventParser(
      aggregatorProgram.programId,
      aggregatorProgram.coder
    );
  }

  static async load(
    programID: PublicKey = CHAINLINK_AGGREGATOR_PROGRAM_ID,
    provider: anchor.Provider = anchor.getProvider()
  ): Promise<OCR2Feed> {
    const aggregatorProgram = await anchor.Program.at(programID, provider);
    return new OCR2Feed(aggregatorProgram, provider);
  }

  public onRound(feed: PublicKey, callback: (round: Round) => void): number {
    return this.provider.connection.onLogs(feed, (event, ctx) => {
      this._parser.parseLogs(event.logs, (log) => {
        if (log.name != "NewTransmission") {
          return;
        }
        callback(OCR2Feed.parseLog(feed, log));
      });
    });
  }

  public async removeListener(listener: number): Promise<void> {
    return this.provider.connection.removeOnLogsListener(listener);
  }

  public static parseLog(feed, log): Round {
    if (!log || !log.data) return null;
    let answer: BN;
    if (log.data.answer)
      answer = new BN(log.data.answer as Uint8Array, 10, "le");
    let roundId: number;
    if (log.data.roundId) roundId = log.data.roundId as number;
    let observationsTS: Date;
    if (log.data.observationsTimestamp)
      observationsTS = new Date(
        (log.data.observationsTimestamp as number) * 1000
      );
    return {
      feed: feed,
      answer: answer,
      roundId: roundId,
      observationsTS: observationsTS,
    };
  }
}
