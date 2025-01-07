package chainwriter

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/utils"
)

type TestArgs struct {
	Inner []InnerArgs
}

type InnerArgs struct {
	Address []byte
}

type DataAccount struct {
	Discriminator        [8]byte
	Version              uint8
	Administrator        solana.PublicKey
	PendingAdministrator solana.PublicKey
	LookupTable          solana.PublicKey
}

func GetDebugIDAtLocation(args any, location string) (string, error) {
	debugIDList, err := utils.GetValuesAtLocation(args, location)
	if err != nil {
		return "", err
	}

	if len(debugIDList) == 0 {
		return "", errors.New("no debug ID found at location: " + location)
	}
	// there should only be one debug ID, others will be ignored.
	debugID := string(debugIDList[0])

	return debugID, nil
}

func errorWithDebugID(err error, debugID string) error {
	if debugID == "" {
		return err
	}
	return fmt.Errorf("Debug ID: %s: Error: %s", debugID, err)
}

func InitializeDataAccount(
	ctx context.Context,
	t *testing.T,
	client *rpc.Client,
	programID solana.PublicKey,
	admin solana.PrivateKey,
	lookupTable solana.PublicKey,
) {
	pda, _, err := solana.FindProgramAddress([][]byte{[]byte("data")}, programID)
	require.NoError(t, err)

	discriminator := GetDiscriminator("initialize_lookup_table")

	instructionData := append(discriminator[:], lookupTable.Bytes()...)

	instruction := solana.NewInstruction(
		programID,
		solana.AccountMetaSlice{
			solana.Meta(pda).WRITE(),
			solana.Meta(admin.PublicKey()).SIGNER().WRITE(),
			solana.Meta(solana.SystemProgramID),
		},
		instructionData,
	)

	// Send and confirm the transaction
	utils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, admin, rpc.CommitmentFinalized)
}

func GetDiscriminator(instruction string) [8]byte {
	fullHash := sha256.Sum256([]byte("global:" + instruction))
	var discriminator [8]byte
	copy(discriminator[:], fullHash[:8])
	return discriminator
}

func GetRandomPubKey(t *testing.T) solana.PublicKey {
	privKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	return privKey.PublicKey()
}

func CreateTestPubKeys(t *testing.T, num int) solana.PublicKeySlice {
	addresses := make([]solana.PublicKey, num)
	for i := 0; i < num; i++ {
		addresses[i] = GetRandomPubKey(t)
	}
	return addresses
}

func CreateTestLookupTable(ctx context.Context, t *testing.T, c *rpc.Client, sender solana.PrivateKey, addresses []solana.PublicKey) solana.PublicKey {
	// Create lookup tables
	slot, serr := c.GetSlot(ctx, rpc.CommitmentFinalized)
	require.NoError(t, serr)
	table, instruction, ierr := utils.NewCreateLookupTableInstruction(
		sender.PublicKey(),
		sender.PublicKey(),
		slot,
	)
	require.NoError(t, ierr)
	utils.SendAndConfirm(ctx, t, c, []solana.Instruction{instruction}, sender, rpc.CommitmentConfirmed)

	// add entries to lookup table
	utils.SendAndConfirm(ctx, t, c, []solana.Instruction{
		utils.NewExtendLookupTableInstruction(
			table, sender.PublicKey(), sender.PublicKey(),
			addresses,
		),
	}, sender, rpc.CommitmentConfirmed)

	return table
}
