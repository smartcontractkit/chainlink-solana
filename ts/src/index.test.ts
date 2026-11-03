import {
  CHAINLINK_AGGREGATOR_PROGRAM_ID,
  CHAINLINK_STORE_PROGRAM_ID,
  OCR2Feed,
  Round,
  StoreFeed,
} from ".";
import { BN } from "@coral-xyz/anchor";
import * as anchor from "@coral-xyz/anchor";
import * as os from "os";
import * as fs from "fs";
import * as path from "path";
import { createHash } from "crypto";

describe("OCR2Feed", () => {
  it("parseLog", () => {
    let got = OCR2Feed.parseLog({
      data: {
        roundId: 1688241,
        configDigest: [
          0, 3, 244, 37, 36, 199, 185, 208, 41, 21, 110, 43, 204, 175, 70, 82,
          5, 68, 97, 146, 252, 172, 91, 34, 130, 116, 27, 155, 249, 67, 170, 5,
        ],
        answer: new BN("328053000000"),
        transmitter: 2,
        observationsTimestamp: 1648768290,
        observerCount: 32,
        observers: [0, 1, 3, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0],
        juelsPerLamport: new BN("136747327"),
        reimbursement: new BN("683736635000"),
      },
    });
    expect(got).toEqual({
      answer: new BN("328053000000"),
      roundId: 1688241,
      observationsTS: new Date("2022-03-31T23:11:30.000Z"),
    });
  });
  it("parseLog null log", () => {
    let got = OCR2Feed.parseLog(null);
    expect(got).toBeNull();
  });
  it("parseLog null data", () => {
    let got = OCR2Feed.parseLog({});
    expect(got).toBeNull();
  });
  it("parseLog empty data", () => {
    let got = OCR2Feed.parseLog({ data: {} });
    expect(got).toBeDefined();
    expect(got.answer).toBeUndefined();
    expect(got.roundId).toBeUndefined();
    expect(got.observationsTS).toBeUndefined();
  });

  let itWS = process.env.ANCHOR_PROVIDER_URL ? it : it.skip;
  itWS(
    "onRound",
    async () => {
      // Temp dir
      let tmpResolve;
      let tmpPromise = new Promise<string>((r) => (tmpResolve = r));
      fs.mkdtemp(path.join(os.tmpdir(), "anchor-wallet-"), (err, folder) => {
        if (err) {
          console.log(err);
          tmpResolve("");
          return;
        }
        tmpResolve(folder);
      });
      let tmpDir = await tmpPromise;
      expect(tmpDir).toBeDefined();

      // Temp key
      let keyPair = anchor.web3.Keypair.generate();
      process.env.ANCHOR_WALLET = path.join(tmpDir, "test-anchor-key");
      let doneResolve;
      let donePromise = new Promise<boolean>((r) => (doneResolve = r));

      let json = JSON.stringify(Array.from(keyPair.secretKey));
      fs.writeFile(process.env.ANCHOR_WALLET, json, (err) => {
        if (err) {
          console.log(err);
          doneResolve(false);
          return;
        }
        doneResolve(true);
      });
      let done = await donePromise;
      expect(done).toBeTruthy();

      // Listen to feed
      let cl = await OCR2Feed.load(
        CHAINLINK_AGGREGATOR_PROGRAM_ID,
        anchor.AnchorProvider.env()
      );
      let resolve;
      let promise = new Promise<Round>((r) => (resolve = r));
      let num;
      num = cl.onRound(CHAINLINK_STORE_PROGRAM_ID, (round) => {
        resolve(round);
        cl.removeListener(num);
      });
      let got = await promise;
      expect(got).toBeDefined();
      expect(got.feed).toEqual(CHAINLINK_STORE_PROGRAM_ID);
      expect(got.answer).toBeDefined();
      expect(got.roundId).toBeDefined();
      expect(got.observationsTS).toBeDefined();
      expect(got.slot).toBeDefined();
    },
    20_000
  );
});

describe("StoreFeed", () => {
  // Program data log of a CacheRoundSubmitted event, as emitted by the store
  const eventLog = (
    feed: anchor.web3.PublicKey,
    roundId: number,
    answer: BN,
    timestamp: number
  ) => {
    const discriminator = createHash("sha256")
      .update("event:CacheRoundSubmitted")
      .digest()
      .subarray(0, 8);
    const round = Buffer.alloc(4);
    round.writeUInt32LE(roundId);
    const ts = Buffer.alloc(4);
    ts.writeUInt32LE(timestamp);
    const data = Buffer.concat([
      discriminator,
      feed.toBuffer(),
      round,
      answer.toTwos(128).toArrayLike(Buffer, "le", 16),
      ts,
    ]);
    return "Program data: " + data.toString("base64");
  };

  it("parseLogs", () => {
    const provider = new anchor.AnchorProvider(
      new anchor.web3.Connection("http://localhost:8899"),
      new anchor.Wallet(anchor.web3.Keypair.generate()),
      {}
    );
    const storeFeed = new StoreFeed(provider);
    const feedA = anchor.web3.Keypair.generate().publicKey;
    const feedB = anchor.web3.Keypair.generate().publicKey;
    const store = CHAINLINK_STORE_PROGRAM_ID.toBase58();

    const rounds = storeFeed.parseLogs([
      `Program ${store} invoke [1]`,
      eventLog(feedA, 7, new BN("1050000000"), 1648768290),
      eventLog(feedB, 3, new BN("-250000000"), 1648768291),
      `Program ${store} success`,
    ]);

    expect(rounds.length).toEqual(2);
    expect(rounds[0].feed.equals(feedA)).toBe(true);
    expect(rounds[0].roundId).toEqual(7);
    expect(rounds[0].answer.toString()).toEqual("1050000000");
    expect(rounds[0].observationsTS).toEqual(
      new Date("2022-03-31T23:11:30.000Z")
    );
    expect(rounds[1].feed.equals(feedB)).toBe(true);
    expect(rounds[1].answer.toString()).toEqual("-250000000");
  });

  it("parseLogs ignores other programs", () => {
    const provider = new anchor.AnchorProvider(
      new anchor.web3.Connection("http://localhost:8899"),
      new anchor.Wallet(anchor.web3.Keypair.generate()),
      {}
    );
    const other = anchor.web3.Keypair.generate().publicKey.toBase58();
    const rounds = new StoreFeed(provider).parseLogs([
      `Program ${other} invoke [1]`,
      eventLog(anchor.web3.Keypair.generate().publicKey, 1, new BN(1), 1),
      `Program ${other} success`,
    ]);
    expect(rounds.length).toEqual(0);
  });

  it("parseLog null log", () => {
    expect(StoreFeed.parseLog(null)).toBeNull();
  });
});
