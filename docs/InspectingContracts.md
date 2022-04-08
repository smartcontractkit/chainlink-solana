# Inspecting Solana Programs

Programs for Solana can be found in the [programs](/contracts/programs/) directory. Here, you can find source code for each of the Chainlink programs deployed on Solana.

- [Access Controller](/contracts/programs/access-controller/)
- [OCR2](/contracts/programs/ocr2/)
- [Store](/contracts/programs/store/)

### Via Solana Explorer

[Solana Explorer](https://explorer.solana.com/) allows users to search for deployed programs and read their state. On the home page, search the address of the program account that you want to inspect. For example, if you want to inspect the OCR2 program for the BTC/USD feed on mainnet, search *2oyA8ZLwuWeAR5ANyDsiEGueUyDC8jFGFLSixSzT9KtV*.

On the program account page, select the *Anchor Account* tab. In this tab, you will be able to inspect all of the data stored on the program in JSON format. In the case of an OCR2 program, this includes on-chain configuration and oracle information.

### Via cURL

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

### Via Solana-CLI

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

### Via Gauntlet Inspect

