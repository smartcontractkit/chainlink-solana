package types

import "github.com/gagliardetto/solana-go"

var (
	SampleTxResultProgram = solana.MustPublicKeyFromBase58("cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ")
	SampleTxResultJSON    = `{"blockTime":1712887149,"meta":{"computeUnitsConsumed":64949,"err":null,"fee":5000,"innerInstructions":[{"index":1,"instructions":[{"accounts":[2,4],"data":"6y43XFem5gk9n8ESJ4pGFboagJiimTtvvy2VCjAUur3y","programIdIndex":3,"stackHeight":2}]}],"loadedAddresses":{"readonly":[],"writable":[]},"logMessages":["Program ComputeBudget111111111111111111111111111111 invoke [1]","Program ComputeBudget111111111111111111111111111111 success","Program cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ invoke [1]","Program data: gjbLTR5rT6hW4eUAAAN30/iLBm0GRKxe6y9hGtvvKCPLmscA16aVgw6AKe17ouFpAAAAAAAAAAAAAAAAA2uVGGYEAwECAAAAAAAAAAAAAAAAAAAAAKom6kICAAAAsr0AAAAAAAA=","Program HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny invoke [2]","Program log: Instruction: Submit","Program HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny consumed 4427 of 140121 compute units","Program HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny success","Program cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ consumed 64799 of 199850 compute units","Program cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ success"],"postBalances":[1019067874127,49054080,2616960,1141440,0,0,1141440,1],"postTokenBalances":[],"preBalances":[1019067879127,49054080,2616960,1141440,0,0,1141440,1],"preTokenBalances":[],"rewards":[],"status":{"Ok":null}},"slot":291748793,"transaction":{"message":{"accountKeys":["9YR7YttJFfptQJSo5xrnYoAw1fJyVonC1vxUSqzAgyjY","Ghm1a2c2NGPg6pKGG3PP1GLAuJkHm1RKMPqqJwPM7JpJ","HXoZZBWv25N4fm2vfSKnHXTeDJ31qaAcWZe3ZKeM6dQv","HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny","3u6T92C2x18s39a7WNM8NGaQK1YEtstTtqamZGsLvNZN","Sysvar1nstructions1111111111111111111111111","cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ","ComputeBudget111111111111111111111111111111"],"header":{"numReadonlySignedAccounts":0,"numReadonlyUnsignedAccounts":5,"numRequiredSignatures":1},"instructions":[{"accounts":[],"data":"3DTZbgwsozUF","programIdIndex":7,"stackHeight":null},{"accounts":[1,0,2,3,4,5],"data":"4W4pS7SH6dugDLwXWijhmW3dGTP7WENQa9vbUjvati1j95ghou2jUJHxvPUoowhZk2bHk21uKk4uFRQrpVF5e54NejQLtAT4DeZPC8n3QudjXhAHgBvFjYvDZDhCKRBK4nvdysDh7aKSE4nb3RiampwUo4u5WsKFfXYZnzbn8edC6jwuJVju1DczQPiLuzuCUps99C8rxwE9XkonGMrjc3Pj4cArMggk5fitRkfdaUn4mGRXDHzPFSg63YTZEn7tnnJd8pWEu9v9H8wBKcN1ptLiY5QmKSnayRcfYvd8MZ9wWf8bD7iVGSNUnwJToyFBVyBNabibozthXSDNmxr3yz1uR9vE3HFq6C2i1LX32a2aqZWzJjmvgdVNfNZZxqDxR6GvWYMw35","programIdIndex":6,"stackHeight":null}],"recentBlockhash":"BKUsMxK39LcgXKm8j5LuYyhig2kgQtRBkxR89szEzaSU"},"signatures":["2eEb8FeJyhczELJ3XKc6yvNLi3jYoC9vdpaR6WUN5vJ3f15ZV1d7LGZZqrqseQFEedgE4cxwcd3S3jYLmvJWBrNg"]}}`
)
