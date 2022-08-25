# Withdraw Payments using Gauntlet

When Node Operators report data to a feed, they are compensated with LINK thats owned by the feed. Node operators can then withdraw the funds owed to them via gauntlet using this section below

## Sample Command Usage

Make sure to set up gauntlet using the steps described in [Gauntlet Setup](gauntlet-setup.md) before attempting to run the following command

```bash
yarn gauntlet ocr2:withdraw_payment --network=mainnet --recipient=<VALID_LINK_RECIPIENT_ADDRESS> <FEED_ADDRESS>
```

If you are using a Ledger, include the `--withLedger` flag. Gauntlet will ask you to sign the transaction using your Ledger.

```bash
yarn gauntlet ocr2:withdraw_payment --network=mainnet --recipient=<VALID_LINK_RECIPIENT_ADDRESS> --withLedger <FEED_ADDRESS>
```
