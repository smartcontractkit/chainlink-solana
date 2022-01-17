import * as anchor from "@project-serum/anchor";
import { BN } from "@project-serum/anchor";
import { Connection, PublicKey } from "@solana/web3.js";

export const CHAINLINK_AGGREGATOR_PROGRAM_ID = new PublicKey("test");
export const CHAINLINK_STORE_PROGRAM_ID = new PublicKey("test");

export class Chainlink {
  private _parser: anchor.EventParser;
  
  /**
   * Constructor for new Chainlink client object
   * @param connection
   * @param config
   * @param state
   */
  constructor(
    readonly aggregatorProgram: anchor.Program,
    readonly provider: anchor.Provider,
  ) {
    this._parser = new anchor.EventParser(aggregatorProgram.programId, aggregatorProgram.coder);
  }
  
  /**
   * Load an onchain Chainlink program.
   *
   * @param connection The connection to use
   * @param feedAccount The public key of the account to load
   * @param programID Address of the onchain Chainlink program
   */
  static async load(
    programID: PublicKey = CHAINLINK_AGGREGATOR_PROGRAM_ID,
    provider?: anchor.Provider,
  ): Promise<Chainlink> {
    provider = provider ?? anchor.getProvider();
    const aggregatorProgram = await anchor.Program.at(programID, provider);
    return new Chainlink(aggregatorProgram, provider);
  }
  
  public onRound(feed: PublicKey, callback: (event: any) => void): number {
    return this.provider.connection.onLogs(feed, (event, ctx) => {
        this._parser.parseLogs(event.logs, (log) => {
          if (log.name != "NewTransmission") {
            return;
          }
          let answer = log.data.answer as Uint8Array;
          let event = {
            ...log.data,
            answer: new BN(answer, 10, 'le'),
            slot: ctx.slot,
          };
          callback(event);
        })
    })
  }
  
  public async removeListener(listener: number): Promise<void> {
    return this.provider.connection.removeOnLogsListener(listener)
  }
}