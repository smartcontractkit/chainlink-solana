package fees

import (
	"fmt"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateFee(t *testing.T) {
	inputs := []struct {
		base, max, min uint64
		count          uint
		expected       uint64
	}{
		{0, 0, 0, 100, 0},    // test max
		{0, 10, 1, 0, 1},     // test min
		{0, 10, 0, 0, 0},     // test 0 count should return base
		{0, 10, 0, 1, 1},     // test 1 count on 0 base should return 1
		{0, 10, 0, 2, 2},     // test 2 count on 0 base should return 2
		{0, 10, 0, 3, 4},     // test 3 count on 0 base should return 4
		{0, 10, 0, 4, 8},     // test 4 count on 0 base should return 8
		{1, 10, 0, 0, 1},     // test 0 count on 1 base should return 1
		{1, 10, 0, 1, 2},     // test 1 count on 1 base should return 2
		{1, 100, 0, 64, 100}, // test 64 bcount on 1 base should return max (overflow)
	}

	for i, v := range inputs {
		t.Run(fmt.Sprintf("inputs[%d]", i), func(t *testing.T) {
			assert.Equal(t, v.expected, CalculateFee(v.base, v.max, v.min, v.count))
		})
	}
}

func TestParseBlock(t *testing.T) {
	testBlocks := readMultipleBlocksFromFile(t, "./multiple_blocks_data.json")
	firstBlock := testBlocks[0]
	assert.Equal(t, 3, len(firstBlock.Transactions))

	// happy path - filtered for non-vote txs
	out, err := ParseBlock(firstBlock)
	require.NoError(t, err)
	assert.Equal(t, len(out.Prices), len(out.Fees))
	assert.Equal(t, 2, len(out.Prices))

	// fail nil
	_, err = ParseBlock(nil)
	require.Error(t, err)

	// skip on nil meta
	out, err = ParseBlock(&rpc.GetBlockResult{
		Transactions: []rpc.TransactionWithMeta{{}},
	})
	require.NoError(t, err)
	assert.Equal(t, 0, len(out.Prices))

	// error on failed tx parsing
	_, err = ParseBlock(&rpc.GetBlockResult{
		Transactions: []rpc.TransactionWithMeta{{
			Transaction: &rpc.DataBytesOrJSON{},
			Meta:        &rpc.TransactionMeta{},
		}},
	})
	assert.Error(t, err)
}

func TestParseTxes(t *testing.T) {
	var (
		v0   = "AWKN1c7lhjktevgsaMIs4Ae7ZccwPVWwW1R+imr2DN/0y7Tkoo9cWDnXwwfjzmf/c/DkM+7lBpwyyfRaEDxd/wuAAQADBtPEnaktbn1X1zsaqA/ykS7EbEaOP/4j0xuy42/xGU5hgBRVEFEqfGiVXkaREkqCLjS1lEtiLHI4nqyNlxqaZyBX0v48x47K0QXEyun42w8fs3FCFMroHi/ku/XtfF1uNMTksGwie+7XMvstV8koVGKgstFJ961xVc8gDzKdDzq3C/RuaMyFoh3pvIOLcuzuIGcpXSPTnHF1pdMdjMSg268DBkZv5SEXMv/srbpyw5vnvIzlu8X3EmssQ5s6QAAAACAcd9uB/0r7tJdDix6EeQLrUDOM55LA5Hezb3MxjTZCAwUACQNSAAAAAAAAAAQOBgcACAkKCwMMDQ4PAQK9ArqRw+PP0+KGAApFHuoHMtB6j08iIxiDJApnzGmdDywRrBtGGQwpAE8AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAB2B4EkAAAABAAAABQ1JQteEk5+F2SIL0wiPFZzeeScGN/BAsqr9S1mFOQMAAAAAAAAAAAAAAAAAAAAAKKNzWCrh5nLBAAAAAAAAAAAAAAAAAgAAACysfJCSXEAI0+ZwJePlArK1yx6Z+WoQIUV3tWnucGF5mNCzQAvjcHvVQvSxS5bkvTBa01gPnUSDC71mlxi4YQECAAAAL+MuvSQrbyblb1MNQjDYHMRPYNt1g1D9A83CMbiIl99Mp/bod4YmyKegvI1GgW4WAs6awsfYW8WkJ6U3wISIfgABAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABQAFAuPrAQABiyLUZuiWwHjgY92WJwfxJ8SDzY4nPRnAtfcKb2dCYXUACgcIAAIJCwoTFRQ="
		v1   = "gQIACg8AAACq2gUre9xa2z/ldC51RithZ9L9aAc+p+sJ6V2MJwe1PAEQAawPKl/5X+e2HmZvgahqRpI32z+068bN1nZXs7QyQnWBrTHEzH6EYXLYouw55rgM6q1dotGE6vsp7CW4gQCyQM+aWYngH1KqcdSSnbSmnC+KD5TvPjXeIxZ1txpehF2rB26u7rNdg35WuF+8qDNV3qMUG5z9W5Cz6MbufAo0lC9eg6nVMcg4+XXkBHoyjS05fVyKshMxQKvtHJFOqaNM1TtELLORIVfxOpM9ATQoLQMrX/7NAaLb8bd5BgjfAC6npl/IHQ/vqIYMs7g/CJsCJL6KZoe3rkn1lMC5tNfpOJMtx71r2jruWGPiFgQi7ixv89aruYChuVkAhwQ4h3atSSe80KmkZRziYm6y2Xn9E0qlwIRiCYWZJIDDOm1AKuwhjmtqMVIduprQivBrbYzpqqz6gh6ncSy7S5JpSm8DNvs1TXS2FccMV1tw/ADXFrs+B7UxAFPod5lElck702jpzsFO0/6/8xuJScnUESApvF0+/1Dasy9BZIvWE5NzM+G/pl/IHOGe3NLSw0CwL6Yb4dW63eFZKDPd+SAJ2M9oVFUG3fbh12Whk9nL4UbO63msHLSF7V9bN5E6jPWFfv8AqQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAUQAz/MeAA/Df1b5nUHSbhLn0gjucyDtstU8a2wzuIG5LAAAAAAAAAEBCDwD5NxYABhJgAAAABwIIAwkKCwQFAQwGDQ4PBtc8PS5yN4CwZAAAAAAAAAABAAAAAAAAAAAAAAAAAAAAhwDpyo5Q+jN1mowJMPpSSVxGv6gAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA0AcAAFftL1crN5PSqsJE9fg0rmM5vDN4PMC2CM9vsYLUkQKCDKc/76wY0cPsFFokTortVQg/MFh8k3Ww8tMjorvcoQEmYor09OP/byoeEvW+LaagcoOSWuS8vXwelWEiQvM6rOatUVmjsTcGD1Nrf9P2GyQkDmH0FNAuF9Q0laadfF8B"
		vote = "Ae/JA+ZhTtB0NvGV3lU1jcwiCuyxgw/LHZHGiKgd7zspL1U4VmOYLXkE2jYnKMTyBHk/23YuGv3O+ABlJj6alggBAAEDOAHGf6ImDGjnQItj2TQgdQUU8QA9ZJZgTbXnc+4a1KRBjZn9yvHMk/MX4P6bHyB5zZ2/JXsxD0sAI9CGqBubRgdhSB01dHS7fE12JOvTvbPYNV5z0RBD/A2jU4AAAAAAST7jThpcniS5mrNhGkOEKK0OyGP8/q8LwOhPrEiIVcwBAgIBAJQBDgAAAHgrhR0AAAAAHwEfAR4BHQEcARsBGgEZARgBFwEWARUBFAETARIBEQEQAQ8BDgENAQwBCwEKAQkBCAEHAQYBBQEEAQMBAgEBJ9b/VLHvTB8Iyf75BY1LqFYS8bT+cV4inIO0UkvTNnYBhWigagAAAAC+sWbhR/lMhTYrWy6pxFArRJHoZiU2OWtSAiG5VvUp1Q=="
	)

	v0Tx, err := solana.TransactionFromBase64(v0)
	require.NoError(t, err)
	v1Tx, err := solana.TransactionFromBase64(v1)
	require.NoError(t, err)
	voteTx, err := solana.TransactionFromBase64(vote)
	require.NoError(t, err)
	require.True(t, isConsensusVoteTX(voteTx))
	require.False(t, isConsensusVoteTX(v0Tx))
	require.False(t, isConsensusVoteTX(v1Tx))
	price0 := parsePriceFromTransactionV0(v0Tx)
	price1 := parsePriceFromTransactionV1(v1Tx)
	expPrice0 := ComputeUnitPrice(82)
	expPrice1 := ComputeUnitPrice(75)
	require.Equal(t, expPrice0, price0)
	require.Equal(t, expPrice1, price1)
}
