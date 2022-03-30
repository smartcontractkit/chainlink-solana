package solana

import (
	"bytes"
	"context"

	"github.com/gagliardetto/solana-go"
	"github.com/pkg/errors"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
)

var _ types.ContractTransmitter = (*ContractTracker)(nil)

// Transmit sends the report to the on-chain OCR2Aggregator smart contract's Transmit method
func (c *ContractTracker) Transmit(
	ctx context.Context,
	reportCtx types.ReportContext,
	report types.Report,
	sigs []types.AttributedOnchainSignature,
) error {
	blockhash, err := c.reader.LatestBlockhash()
	if err != nil {
		return errors.Wrap(err, "error on Transmit.GetRecentBlockhash")
	}
	if blockhash == nil || blockhash.Value == nil {
		return errors.New("nil pointer returned from Transmit.GetRecentBlockhash")
	}

	// Determine store authority
	seeds := [][]byte{[]byte("store"), c.StateID.Bytes()}
	storeAuthority, storeNonce, err := solana.FindProgramAddress(seeds, c.ProgramID)
	if err != nil {
		return errors.Wrap(err, "error on Transmit.FindProgramAddress")
	}

	if _, err = c.ReadState(); err != nil {
		return errors.Wrap(err, "error on Transmit.ReadState")
	}
	accounts := []*solana.AccountMeta{
		// state, transmitter, transmissions, store_program, store, store_authority
		{PublicKey: c.StateID, IsWritable: true, IsSigner: false},
		{PublicKey: c.Transmitter.PublicKey(), IsWritable: false, IsSigner: true},
		{PublicKey: c.TransmissionsID, IsWritable: true, IsSigner: false},
		{PublicKey: c.StoreProgramID, IsWritable: false, IsSigner: false},
		{PublicKey: storeAuthority, IsWritable: false, IsSigner: false},
	}

	reportContext := RawReportContext(reportCtx)

	// Construct the instruction payload
	data := new(bytes.Buffer) // store_nonce || report_context || raw_report || raw_signatures
	data.WriteByte(storeNonce)
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
		blockhash.Value.Blockhash,
		solana.TransactionPayer(c.Transmitter.PublicKey()),
	)
	if err != nil {
		return errors.Wrap(err, "error on Transmit.NewTransaction")
	}

	msgToSign, err := tx.Message.MarshalBinary()
	if err != nil {
		return errors.Wrap(err, "error on Transmit.Message.MarshalBinary")
	}
	finalSigBytes, err := c.Transmitter.Sign(msgToSign)
	if err != nil {
		return errors.Wrap(err, "error on Transmit.Sign")
	}
	var finalSig [64]byte
	copy(finalSig[:], finalSigBytes)
	tx.Signatures = append(tx.Signatures, finalSig)

	// pass transmit payload to tx manager queue
	c.lggr.Debugf("Queuing transmit tx: state (%s) + transmissions (%s)", c.StateID.String(), c.TransmissionsID.String())
	err = c.txManager.Enqueue(c.StateID.String(), tx)
	return errors.Wrap(err, "error on Transmit.txManager.Enqueue")
}

func (c *ContractTracker) LatestConfigDigestAndEpoch(
	ctx context.Context,
) (
	configDigest types.ConfigDigest,
	epoch uint32,
	err error,
) {
	state, err := c.ReadState()
	return state.Config.LatestConfigDigest, state.Config.Epoch, err
}

func (c *ContractTracker) FromAccount() types.Account {
	return types.Account(c.Transmitter.PublicKey().String())
}
