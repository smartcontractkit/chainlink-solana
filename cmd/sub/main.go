package main

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go/rpc"
)

func main() {
	rpcEndpoint := "https://floral-morning-sun.solana-devnet.quiknode.pro/d874b0e33834d6babaa1e60a5b6181f22dd0409e/"

	accountPubKeyBase58 := "HoLknTuGPcjsVDyEAu92x1njFKc5uUXuYLYFuhiEatF1" // testnet LINK/USD transmissions
	//accountPubKeyBase58 := "2TQmhSnGK5NwdXBKEmJ8wfwH17rNSQgH11SVdHkYC1ZD" // testnet LINK/USD state
	//accountPubKeyBase58 := "STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3" // testnet LINK/USD program address
	accountPubKey := solana.MustPublicKeyFromBase58(accountPubKeyBase58)

	ctx := context.Background()
	client := rpc.New(rpcEndpoint)

	finalized, err := client.GetBlockHeight(ctx, rpc.CommitmentFinalized)
	if err != nil {
		panic(err)
	}
	//confirmed, err := client.GetBlockHeight(ctx, rpc.CommitmentConfirmed)
	//if err != nil {
	//	panic(err)
	//}
	//processed, err := client.GetBlockHeight(ctx, rpc.CommitmentProcessed)
	//if err != nil {
	//	panic(err)
	//}
	fmt.Println(">>>>>>>> finalized", finalized)
	//fmt.Println(">>>>>>>> confirmed", confirmed)
	//fmt.Println(">>>>>>>> processed", processed)

	client.GetSignaturesForAddressWithOpts(ctx, accountPubKey
}

/*
func main() {
	ctx := context.Background()
	//wsEndpoint := "wss://4OFGJRWARF3I3HOX6ARO:VDIMBJ7OBPV5N2KWYRNNFEKEYWEQ4ODLINA7MQRR@ac135df3-af7f-45e6-9204-4c311edaf467.solana.bison.run/ws" // MALFORMED mainnet
	//wsEndpoint := "wss://wispy-bold-water.solana-mainnet.quiknode.pro/01b51251bd130abae974c0cc72d79f068c133416/" // mainnet
	wsEndpoint := "wss://floral-morning-sun.solana-devnet.quiknode.pro/d874b0e33834d6babaa1e60a5b6181f22dd0409e/" // testnet

	client, err := ws.Connect(ctx, wsEndpoint)
	if err != nil {
		panic(err)
	}

	//accountPubKeyBase58 := "HoLknTuGPcjsVDyEAu92x1njFKc5uUXuYLYFuhiEatF1" // testnet LINK/USD transmissions
	//accountPubKeyBase58 := "2TQmhSnGK5NwdXBKEmJ8wfwH17rNSQgH11SVdHkYC1ZD" // testnet LINK/USD state
	accountPubKeyBase58 := "STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3" // testnet LINK/USD program address
	accountPubKey := solana.MustPublicKeyFromBase58(accountPubKeyBase58)
	//commitment := rpc.CommitmentConfirmed // No values received
	commitment := rpc.CommitmentProcessed

	//sub, err := client.AccountSubscribe(accountPubKey, commitment)
	//sub, err := client.ProgramSubscribe(accountPubKey, commitment)
	sub, err := client.LogsSubscribeMentions(accountPubKey, commitment)
	if err != nil {
		panic(err)
	}

	for {
		res, err := sub.Recv()
		if err != nil {
			panic(err)
		}
		//	// for account subscriptions
		//	fmt.Println(">>>>>>>>>>>>>>")
		//	fmt.Println("slot", res.Context.Slot)
		//	fmt.Println("lamports", res.Value.Account.Lamports)
		//	fmt.Println("owner", res.Value.Account.Owner)
		//	fmt.Println("executable", res.Value.Account.Executable)
		//	fmt.Println("rent-epoch", res.Value.Account.RentEpoch)
		//	if res.Value.Account.Data != nil {
		//		fmt.Println("data", res.Value.Account.Data.GetBinary()[:100])
		//		fmt.Println("data size", len(res.Value.Account.Data.GetBinary()))
		//	}
		//	fmt.Println(">>>>>>>>>>>>>>")
		// for program subscriptions
		//fmt.Println(">>>>>>>>>>>>>>", res)
		// for logs
		fmt.Println(">>>>>>>>>>>>>>")
		fmt.Println("Slot", res.Context.Slot)
		fmt.Println("Signature", res.Value.Signature[:])
		fmt.Println("Err", res.Value.Err)
		for i, log := range res.Value.Logs {
			fmt.Printf("Log %d: %s\n", i, log)
			if strings.HasPrefix(log, "Program log:") && !strings.HasPrefix(log, "Program log: Instruction:") {
				payload := strings.TrimPrefix(log, "Program log: ")
				fmt.Printf("Payload %d: %#v\n", i, payload)
				//buf, err := base64.RawStdEncoding.DecodeString(payload)
				//buf, err := base64.RawURLEncoding.DecodeString(payload)
				buf, err := base64.StdEncoding.DecodeString(payload)
				//buf, err := base64.URLEncoding.DecodeString(payload)
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Println("Decoded:", string(buf))
				}
				panic("at the disco")
			}
		}
		//fmt.Printf("Logs %#v\n", res.Value.Logs)
		fmt.Println(">>>>>>>>>>>>>>")
	}
}
*/
