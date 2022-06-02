import * as anchor from "@project-serum/anchor";
import { BN } from "@project-serum/anchor";
import { PublicKey } from "@solana/web3.js";

export const CHAINLINK_AGGREGATOR_PROGRAM_ID = new PublicKey(
  "cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ"
);
export const CHAINLINK_STORE_PROGRAM_ID = new PublicKey(
  "HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny"
);

export interface Round {
  feed: PublicKey;
  answer: BN;
  roundId: number;
  observationsTS: Date;

  slot: number;
}

export class OCR2Feed {
  private _parser: anchor.EventParser;

  constructor(
    readonly aggregatorProgram: anchor.Program,
    readonly provider: anchor.AnchorProvider
  ) {
    this._parser = new anchor.EventParser(
      aggregatorProgram.programId,
      aggregatorProgram.coder
    );
  }

  static async load(
    programID: PublicKey = CHAINLINK_AGGREGATOR_PROGRAM_ID,
    provider: anchor.AnchorProvider = anchor.AnchorProvider.env()
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
        let parsed = OCR2Feed.parseLog(log);
        parsed.feed = feed;
        parsed.slot = ctx.slot;
        callback(parsed);
      });
    });
  }

  public async removeListener(listener: number): Promise<void> {
    return this.provider.connection.removeOnLogsListener(listener);
  }

  public static parseLog(log): Round {
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
      answer: answer,
      roundId: roundId,
      observationsTS: observationsTS,
    } as Round;
  }
}
