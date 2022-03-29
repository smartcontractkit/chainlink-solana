import {CHAINLINK_AGGREGATOR_PROGRAM_ID, CHAINLINK_STORE_PROGRAM_ID, OCR2Feed, Round} from '.';
import {Provider} from "@project-serum/anchor";

describe('OCR2Feed', () => {
    //TODO parse tests

    let itWS = process.env.ANCHOR_PROVIDER_URL ? it : it.skip
    itWS(
        'onRound',
        async () => {
            let cl = await OCR2Feed.load(CHAINLINK_AGGREGATOR_PROGRAM_ID, Provider.env())
            let feed = CHAINLINK_STORE_PROGRAM_ID;
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
        120_000,
    )
})
