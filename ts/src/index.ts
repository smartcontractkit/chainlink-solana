import * as anchor from "@coral-xyz/anchor";
import { BN } from "@coral-xyz/anchor";
import { PublicKey } from "@solana/web3.js";

export const CHAINLINK_AGGREGATOR_PROGRAM_ID = new PublicKey(
  "cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ"
);
/** Mainnet / typical public deployment. Local `anchor test` loads `store.so` at the id in `contracts/Anchor.toml`. */
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
      for (const log of this._parser.parseLogs(event.logs)) {
        if (log.name != "newTransmission") {
          return;
        }
        let parsed = OCR2Feed.parseLog(log);
        parsed.feed = feed;
        parsed.slot = ctx.slot;
        callback(parsed);
      }
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

/**
 * Minimal store IDL (Anchor 0.29 format) holding the event the store emits when the data feeds
 * cache (CRE) writes a round into a feed account.
 */
const STORE_EVENTS_IDL = {
  version: "1.0.1",
  name: "store",
  instructions: [],
  events: [
    {
      name: "CacheRoundSubmitted",
      fields: [
        { name: "feed", type: "publicKey", index: false },
        { name: "roundId", type: "u32", index: false },
        { name: "answer", type: "i128", index: false },
        { name: "timestamp", type: "u32", index: false },
      ],
    },
  ],
} as anchor.Idl;

/**
 * Listens for rounds written to a store feed account by the data feeds cache. Once a feed's writer
 * moves from OCR2 to the cache, OCR2 `NewTransmission` events stop and these take their place.
 */
export class StoreFeed {
  private _parser: anchor.EventParser;

  constructor(
    readonly provider: anchor.AnchorProvider = anchor.AnchorProvider.env(),
    readonly storeProgramId: PublicKey = CHAINLINK_STORE_PROGRAM_ID
  ) {
    this._parser = new anchor.EventParser(
      storeProgramId,
      new anchor.BorshCoder(STORE_EVENTS_IDL)
    );
  }

  /** `feed` is the feed (transmissions) account customers read. */
  public onRound(feed: PublicKey, callback: (round: Round) => void): number {
    return this.provider.connection.onLogs(feed, (event, ctx) => {
      if (event.err) return;

      for (const round of this.parseLogs(event.logs)) {
        // one cache report can write several feeds
        if (!round.feed.equals(feed)) continue;
        round.slot = ctx.slot;
        callback(round);
      }
    });
  }

  public async removeListener(listener: number): Promise<void> {
    return this.provider.connection.removeOnLogsListener(listener);
  }

  public parseLogs(logs: string[]): Round[] {
    const rounds: Round[] = [];
    for (const log of this._parser.parseLogs(logs)) {
      if (log.name != "CacheRoundSubmitted") continue;
      const parsed = StoreFeed.parseLog(log);
      if (parsed) rounds.push(parsed);
    }
    return rounds;
  }

  public static parseLog(log): Round {
    if (!log || !log.data) return null;
    return {
      feed: log.data.feed as PublicKey,
      answer: log.data.answer as BN,
      roundId: log.data.roundId as number,
      observationsTS: new Date((log.data.timestamp as number) * 1000),
    } as Round;
  }
}
