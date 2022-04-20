# Inspecting Solana Programs

Programs for Solana can be found in the [programs](/contracts/programs/) directory. Here, you can find source code for each of the Chainlink programs deployed on Solana.

- [Access Controller](/contracts/programs/access-controller/)
- [OCR2](/contracts/programs/ocr2/)
- [Store](/contracts/programs/store/)

## Via Gauntlet Inspect

Using Gaunlet, you can inspect the state of Chainlink programs on Solana.

### Inspect Ownership

Gauntlet offers an *inspect_ownership* command for each program. The following is a template for these commands, where PROGRAM_NAME is either ocr2 or access_controller and NETWORK_NAME corresponds to one of the networks defined in the [networks](/gauntlet/packages/gauntlet-solana-contracts/networks/) folder.

```bash
yarn gauntlet [PROGRAM_NAME]:inspect_ownership --network=[NETWORK_NAME] [PROGRAM_ACCOUNT_ADDRESS]

e.g.
yarn gauntlet ocr2:inspect_ownership --network=mainnet 2oyA8ZLwuWeAR5ANyDsiEGueUyDC8jFGFLSixSzT9KtV
```

This command logs the owner and proposed owner (if defined) of the program account.

```bash
🧤  gauntlet 0.1.2
ℹ️   Loading Local wallet
Operator address is H2ScWiFt1ZMRR1beYWB6Yr9cuADJ8sQhkGvYJxjJNAh8
ℹ️   Checking owner of 2TQmhSnGK5NwdXBKEmJ8wfwH17rNSQgH11SVdHkYC1ZD
ℹ️   Owner: 2CbCTf2V95kMfNA31yYaqJ9oVX7MN71RU6zvvg27PgSz
ℹ️   Proposed Owner: 11111111111111111111111111111111
✨  Done in 12.38s.
```

### OCR2 Inspect

Using Gauntlet, you can query an OCR2 program to obtain information about its oracles and configuration. The following template is used for this command.

```bash
yarn gauntlet ocr2:inspect --network=[NETWORK_NAME] [PROGRAM_ACCOUNT_ADDRESS]

e.g.
yarn gauntlet ocr2:inspect --network=devnet 2TQmhSnGK5NwdXBKEmJ8wfwH17rNSQgH11SVdHkYC1ZD
```

You can find an example response below.

```bash
🧤  gauntlet 0.1.2
ℹ️   Loading Local wallet
Operator address is H2ScWiFt1ZMRR1beYWB6Yr9cuADJ8sQhkGvYJxjJNAh8
ℹ️   Oracle Info:
          - Transmitter: 6s1wr4fv2GdTkcKxnXRdR429FYadX4mUVn9TMMhh3eMz
          - Proposed Payee: 11111111111111111111111111111111
          - From Round ID: 0
          - Payment Gjuels: 223296045
      
.
.
.
      
ℹ️   Oracle Info:
          - Transmitter: 7o1AzTiXbvhZH1UV6ZHA664h4hBFbPbFYdLfSnVprRKa
          - Proposed Payee: 11111111111111111111111111111111
          - From Round ID: 0
          - Payment Gjuels: 222637143
      
ℹ️   Min Answer: 1
ℹ️   Max Answer: 100000000000000
ℹ️   Transmission Payment: 1
ℹ️   Observation Payment: 1
ℹ️   Requester Access Controller: 5vkHdxPiTyfY5VdRpPu8tNTpeik6Cy93M6GzztMPWfAk
ℹ️   Billing Access Controller: CBWXiwPGX6ykWtPGXp4cuFJP53SW81pe9q1YfUASWC46
✨  Done in 12.61s.
```

### OCR2 Inspect Responses

Gauntlet also enables you to view the latest transmission from a given OCR2 program. Transmissions provide information about the current aggregator round, the latest data feed value, the transmitter, and the number of observers that contributed to the transmission. Run the following command.

```bash
yarn gauntlet ocr2:inspect:responses --network=[NETWORK_NAME] [AGGREGATOR_ADDRESS]

e.g. yarn gauntlet ocr2:inspect:responses --network=mainnet 2oyA8ZLwuWeAR5ANyDsiEGueUyDC8jFGFLSixSzT9KtV
```

The response provides the latest on-chain config, the contents of the latest transmission, and the oracles that did not contribute to the transmission.

```bash
🧤  gauntlet 0.1.2
ℹ️   Loading Local wallet
Operator address is 3iXMGCiHZ5UcND8XrRgChwFSHKK3XGagvH53YKozwBm2
ℹ️   Latest Config: 
    - Latest Transmitter: DDws22Z91d3ZzxPFCqvh1BWZY1zyZzLzGHVXXQw5bhwc
    - Latest Aggregator Round ID: 213551
    - Latest Config Digest: 0,3,132,97,236,248,39,85,125,146,34,75,83,125,202,151,233,196,131,119,24,115,248,41,143,88,20,118,17,83,100,137
    - Latest Config Block Number: 129616565
    
ℹ️   Latest Transmission
    - Round Id: 213551
    - Config Digest: 0,3,132,97,236,248,39,85,125,146,34,75,83,125,202,151,233,196,131,119,24,115,248,41,143,88,20,118,17,83,100,137
    - Answer: 4129582000000
    - Transmitter: DDws22Z91d3ZzxPFCqvh1BWZY1zyZzLzGHVXXQw5bhwc
    - Observations Timestamp: 1650467589
    - Observer Count: 12
    - Observers: 3QioEt3JQEs7Sd2B19oBjXpCV2bth8svTmjNsqCwEkux,6FpWeQow6SX9ABRMPgdW948fuoMMbEDWWieoPyMuWEVv,HQUygbE1xW1JTiQSMxds3VcPe5ZjqzUrCE9gEaweohKK,277Xi9qxS7HtewZnXtETX7WmNNeJjcsDWErVnTd5haoi,D5shgkAbSHH1VGDybY5bEbgbvvCMbop4u5WKTKxb3cFq,5od8t5kD2gVZvbMfYUJTfjy2FdqsAEkd8EeLvcaWQqBG,6rRiMihF7UdJz25t5QvS7PgP9yzfubN7TBRv26ZBVAhE,DDws22Z91d3ZzxPFCqvh1BWZY1zyZzLzGHVXXQw5bhwc,BwEkdn8SgNQZkZJhEQStmv4MPEZtqHjurKVxJycGRYLm,Fhfv8uB5Sux1nWiw4ssDrbDdt26BPB4tfoW4Bm2on3rj,9xmyHHdJvryP82a1QwzcAueFhGHG9NfdfSdPp64xduCG,6fhjrYHYfhymJsmwLaRBKZXMK2ChYErkteE7jK6h236e
    - Juels Per Lamport: 7588976109
    - Reimbursement Gjuels: 37944
        
ℹ️   12/16 oracles are responding
        
❌  Oracle 5dDyfdy9fannAdHEkYghgQpiPZrQPHadxBLa1WsGHPFi not responding
            
❌  Oracle 7H2aMNigJrHi5TtXtDtEN9NFQRp5x7GQR48SUWdZ7SnW not responding
            
❌  Oracle BEMZ2yGTwLfxb1tmWHJzURaDSwobp5L6Z6JKMATMyYK3 not responding
            
❌  Oracle Gb2tVHqnEmXEqcqdv5LSwgEusEyP3Yhwy9Gu7D1VBS9z not responding
            
✨  Done in 11.92s.
```

## Via Block Explorers

[Solana Explorer](https://explorer.solana.com/) allows users to search for deployed programs and read their state. On the home page, search the address of the program account that you want to inspect. For example, if you want to inspect the OCR2 program for the BTC/USD feed on mainnet, search *2oyA8ZLwuWeAR5ANyDsiEGueUyDC8jFGFLSixSzT9KtV*.

On the program account page, select the *Anchor Account* tab. In this tab, you will be able to inspect all of the data stored on the program in JSON format. In the case of an OCR2 program, this includes on-chain configuration and oracle information.

Other block explorers include:

- [Solscan](https://solscan.io/)
- [Solana Beach](https://solanabeach.io/)

## Via cURL

You can also interact with deployed programs via cURL. Querying account info using cURL requires the following format.

```bash
curl [RPC_URL] -X POST -H "Content-Type: application/json" -d '
  {
    "jsonrpc": "2.0",
    "id": 1,
    "method": "getAccountInfo",
    "params": [
      "ACCOUNT_ADDRESS",
      {
        "encoding": "base58"
      }
    ]
  }
'
```

You may need to change the encoding if the data stored by your program is larger than 128 bytes. Here is the result of the above query.

```bash
{
    "jsonrpc":"2.0",
    "result": {
        "context": {
            "slot":128841900
        },
        "value": {
            "data": ["BASE_58_STRING...","base58"],
            "executable":false,
            "lamports":49054080,
            "owner":"cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ",
            "rentEpoch":298
        }
    },
    "id":1
}
```

## Via Solana-CLI

Using [solana-cli](https://docs.solana.com/cli/install-solana-cli-tools) produces similar results to cURL. Run the following command to get information about a program account.

```bash
solana account [ACCOUNT_ADDRESS]
```

This command will dump information about the given account.

```bash
Public Key: 2oyA8ZLwuWeAR5ANyDsiEGueUyDC8jFGFLSixSzT9KtV
Balance: 0.04905408 SOL
Owner: cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ
Executable: false
Rent Epoch: 298
Length: 6920 (0x1b08) bytes
0000:   d8 92 6b 5e  68 4b b6 b1  01 f9 00 00  00 00 00 00   ..k^hK..........
0010:   a7 7a 9a 54  2d bb 05 64  37 03 f0 e4  90 be e4 d1   .z.T-..d7.......
0020:   bf 25 69 0f  f1 2a 5d 82  52 57 16 71  f7 8a e3 40   .%i..*].RW.q...@
.
.
.
```
