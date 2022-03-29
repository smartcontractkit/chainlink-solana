import { OCR2Feed, Round} from '.';
import { PublicKey } from "@solana/web3.js";
import * as anchor from "@project-serum/anchor";
import * as os from "os";
import * as fs from "fs";
import * as path from "path";

describe('OCR2Feed', () => {
    //TODO parse tests

    let itWS = process.env.ANCHOR_PROVIDER_URL ? it : it.skip
    let programID = new PublicKey("STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3")
    let feed = new PublicKey("HoLknTuGPcjsVDyEAu92x1njFKc5uUXuYLYFuhiEatF1");
    itWS(
        'onRound',
        async () => {
            // Temp dir
            let tmpResolve
            let tmpPromise = new Promise<string>((r) => (tmpResolve = r))
            fs.mkdtemp(path.join(os.tmpdir(), 'anchor-wallet-'), (err, folder) => {
                if (err) {
                    console.log(err)
                    tmpResolve("")
                    return
                }
                tmpResolve(folder)
            })
            let tmpDir = await tmpPromise
            expect(tmpDir).toBeDefined()

            // Temp key
            let keyPair = anchor.web3.Keypair.generate()
            process.env.ANCHOR_WALLET = path.join(tmpDir,"test-anchor-key")
            let doneResolve
            let donePromise = new Promise<boolean>((r)=>(doneResolve = r))

            let json = JSON.stringify(Array.from(keyPair.secretKey))
            fs.writeFile(process.env.ANCHOR_WALLET, json, (err) => {
                if (err) {
                    console.log(err)
                    doneResolve(false)
                    return
                }
                doneResolve(true)
            })
            let done = await donePromise
            expect(done).toBeTruthy()

            // Listen to feed
            let cl = await OCR2Feed.load(programID, anchor.Provider.env())
            let resolve
            let promise = new Promise<Round>((r) => (resolve = r))
            let num
            num = cl.onRound(feed, (round) => {
                resolve(round)
                cl.removeListener(num);
            })
            let got = await promise
            expect(got).toBeDefined()
            expect(got.contract).toEqual(feed)
            expect(got.answer).toBeDefined()
            expect(got.roundId).toBeDefined()
            expect(got.epoch).toBeDefined()
            expect(got.aggregatorRoundId).toBeDefined()
            expect(got.observationsTS).toBeDefined()
        },
        20_000,
    )
})
