package solana

import (
	"bytes"
	"context"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	confirm "github.com/gagliardetto/solana-go/rpc/sendAndConfirmTransaction"
	"github.com/pkg/errors"

	"github.com/smartcontractkit/libocr/offchainreporting2/types"
)

var _ types.ContractTransmitter = (*ContractTracker)(nil)

// TODO: Blaz where do we get this info from, contract?
var ValidatorProgramID solana.PublicKey = solana.PublicKeyFromBytes([]byte("test"))

// Transmit sends the report to the on-chain OCR2Aggregator smart contract's Transmit method
func (c ContractTracker) Transmit(
	ctx context.Context,
	reportCtx types.ReportContext,
	report types.Report,
	sigs []types.AttributedOnchainSignature,
) error {
	recent, err := c.client.rpc.GetRecentBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return err
	}

	// Determine validator authority
	seeds := [][]byte{[]byte("validator"), c.StateID.Bytes()}
	validatorAuthority, validatorNonce, err := solana.FindProgramAddress(seeds, c.ProgramID)
	if err != nil {
		return err
	}

	// Resolve validator's access controller
	var validator Validator
	if err := c.client.rpc.GetAccountDataInto(ctx, c.state.Config.Validator, &validator); err != nil {
		return err
	}

	accounts := []*solana.AccountMeta{
		// state, transmitter, transmissions, validator_program, validator, validator_authority, validator_access_controller
		{PublicKey: c.StateID, IsWritable: true, IsSigner: false},
		{PublicKey: c.Transmitter.PublicKey(), IsWritable: true, IsSigner: true},
		{PublicKey: c.TransmissionsID, IsWritable: true, IsSigner: false},
		{PublicKey: ValidatorProgramID, IsWritable: false, IsSigner: false},
		{PublicKey: c.state.Config.Validator, IsWritable: true, IsSigner: false},
		{PublicKey: validatorAuthority, IsWritable: false, IsSigner: false},
		{PublicKey: validator.RaisingAccessController, IsWritable: false, IsSigner: false},
	}

	reportContext := RawReportContext(reportCtx)

	// Construct the instruction payload
	data := new(bytes.Buffer) // validator_nonce || report_context || raw_report || raw_signatures
	data.WriteByte(validatorNonce)
	data.Write(reportContext[0][:])
	data.Write(reportContext[1][:])
	data.Write(reportContext[2][:])
	data.Write([]byte(report))
	for _, sig := range sigs {
		// Signature = 64 bytes + 1 byte recovery id
		data.Write(sig.Signature)
	}

	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			solana.NewInstruction(c.ProgramID, accounts, data.Bytes()),
		},
		recent.Value.Blockhash,
		solana.TransactionPayer(c.Transmitter.PublicKey()),
	)
	if err != nil {
		return err
	}

	pkGetter := func(key solana.PublicKey) *solana.PrivateKey {
		if c.Transmitter.PublicKey().Equals(key) {
			return &c.Transmitter
		}
		return nil
	}
	if _, err = tx.Sign(pkGetter); err != nil {
		return errors.Wrap(err, "error on transaction sign")
	}

	// Send transaction, and wait for confirmation:
	_, err = confirm.SendAndConfirmTransaction(
		ctx,
		c.client.rpc,
		c.client.ws,
		tx,
	)

	return err
}

func (c ContractTracker) LatestConfigDigestAndEpoch(
	ctx context.Context,
) (
	configDigest types.ConfigDigest,
	epoch uint32,
	err error,
) {
	err = fetchWrap(ctx, c.fetchState, &c.lockStateChan)
	return c.state.Config.LatestConfigDigest, c.state.Config.Epoch, err
}

func (c ContractTracker) FromAccount() types.Account {
	return types.Account(c.Transmitter.PublicKey().String())
}
