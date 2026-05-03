package solana

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/gagliardetto/solana-go"
)

// pkg/solana cannot import contracts/generated/keystone_forwarder: contracts/ is a separate Go module
// (see contracts/go.mod). Keep these in sync with contracts/generated/keystone_forwarder/{program_id,discriminators}.go.
var (
	keystoneForwarderProgramID = solana.MustPublicKeyFromBase58("whV7Q5pi17hPPyaPksToDw1nMx6Lh8qmNWKFaLRQ4wz")
	// Anchor instruction discriminator for keystone_forwarder::report.
	keystoneForwarderInstructionReportDiscriminator = [8]byte{96, 121, 245, 84, 178, 45, 48, 91}
)

// keystoneForwarderReportFixedRoles label the Anchor `report` instruction account metas in order
// (see contracts/generated/keystone_forwarder/instructions.go NewReportInstruction). These match the
// on-chain PDAs / roles operators care about (oracles config PDA, execution state PDA, forwarder authority PDA, etc.).
var keystoneForwarderReportFixedRoles = []string{
	"ForwarderState",
	"OraclesConfigPDA",
	"Transmitter",
	"ForwarderAuthorityPDA",
	"ExecutionState",
	"ReceiverProgram",
	"SystemProgram",
}

// keystoneForwarderReportSlotLabels returns human-readable slots for this message key index
// when it appears in a compiled instruction to keystone_forwarder::report.
// Slots 0..6 match the IDL; further accounts are CPI "remaining accounts" for the receiver program
// (see keystone-forwarder Report context remaining_accounts).
func keystoneForwarderReportSlotLabels(msg solana.Message, globalKeyIndex int) []string {
	var labels []string
	for ixIdx, ix := range msg.Instructions {
		if int(ix.ProgramIDIndex) >= len(msg.AccountKeys) {
			continue
		}
		prog := msg.AccountKeys[ix.ProgramIDIndex]
		if !prog.Equals(keystoneForwarderProgramID) {
			continue
		}
		if len(ix.Data) < 8 || !bytes.Equal(ix.Data[:8], keystoneForwarderInstructionReportDiscriminator[:]) {
			continue
		}
		for slot, accIx := range ix.Accounts {
			if int(accIx) != globalKeyIndex {
				continue
			}
			labels = append(labels, formatKeystoneForwarderReportSlot(ixIdx, slot))
		}
	}
	return labels
}

// keystoneForwarderReceiverProgramFromMessage returns the receiver_program account pubkey
// from the first compiled keystone_forwarder::report instruction, if present.
func keystoneForwarderReceiverProgramFromMessage(msg solana.Message) solana.PublicKey {
	const receiverSlot = 5
	for _, ix := range msg.Instructions {
		if int(ix.ProgramIDIndex) >= len(msg.AccountKeys) {
			continue
		}
		if !msg.AccountKeys[ix.ProgramIDIndex].Equals(keystoneForwarderProgramID) {
			continue
		}
		if len(ix.Data) < 8 || !bytes.Equal(ix.Data[:8], keystoneForwarderInstructionReportDiscriminator[:]) {
			continue
		}
		if len(ix.Accounts) <= receiverSlot {
			continue
		}
		g := int(ix.Accounts[receiverSlot])
		if g < 0 || g >= len(msg.AccountKeys) {
			continue
		}
		return msg.AccountKeys[g]
	}
	return solana.PublicKey{}
}

func formatKeystoneForwarderReportSlot(ixIdx, slot int) string {
	if slot >= 0 && slot < len(keystoneForwarderReportFixedRoles) {
		return fmt.Sprintf("ix%d:%s", ixIdx, keystoneForwarderReportFixedRoles[slot])
	}
	rem := slot - len(keystoneForwarderReportFixedRoles)
	return fmt.Sprintf("ix%d:ReceiverCPIAccount_%d", ixIdx, rem)
}

// keystoneRoleFromReportSlotLabel strips the "ixN:" prefix from formatKeystoneForwarderReportSlot output.
func keystoneRoleFromReportSlotLabel(label string) string {
	i := strings.IndexByte(label, ':')
	if i < 0 || i+1 >= len(label) {
		return ""
	}
	return label[i+1:]
}

// keystoneForwarderMissingRoleSummary is top-level log metadata when getMultipleAccounts is nil for
// accounts that appear in a keystone_forwarder::report instruction (see Anchor Report<'info>).
type keystoneForwarderMissingRoleSummary struct {
	MissingForwarderState           bool     `json:"missingForwarderState"`
	MissingOraclesConfigPDA         bool     `json:"missingOraclesConfigPDA"`
	MissingTransmitter              bool     `json:"missingTransmitter"`
	MissingForwarderAuthorityPDA    bool     `json:"missingForwarderAuthorityPDA"`
	MissingExecutionState           bool     `json:"missingExecutionState"`
	MissingReceiverProgram          bool     `json:"missingReceiverProgram"`
	MissingSystemProgram            bool     `json:"missingSystemProgram"`
	MissingReceiverCPIAccount       bool     `json:"missingReceiverCPIAccount"`
	MissingKeystoneReportRolesDedup []string `json:"missingKeystoneReportRolesDedup,omitempty"`
}

func (s keystoneForwarderMissingRoleSummary) any() bool {
	return s.MissingForwarderState || s.MissingOraclesConfigPDA || s.MissingTransmitter ||
		s.MissingForwarderAuthorityPDA || s.MissingExecutionState || s.MissingReceiverProgram ||
		s.MissingSystemProgram || s.MissingReceiverCPIAccount
}

func keystoneForwarderSummarizeMissingReportRoles(details []submitTransactionMissingAccount) keystoneForwarderMissingRoleSummary {
	var s keystoneForwarderMissingRoleSummary
	seen := make(map[string]struct{})
	for _, d := range details {
		for _, label := range d.KeystoneForwarderReportSlots {
			role := keystoneRoleFromReportSlotLabel(label)
			if role == "" {
				continue
			}
			seen[role] = struct{}{}
			switch role {
			case "ForwarderState":
				s.MissingForwarderState = true
			case "OraclesConfigPDA":
				s.MissingOraclesConfigPDA = true
			case "Transmitter":
				s.MissingTransmitter = true
			case "ForwarderAuthorityPDA":
				s.MissingForwarderAuthorityPDA = true
			case "ExecutionState":
				s.MissingExecutionState = true
			case "ReceiverProgram":
				s.MissingReceiverProgram = true
			case "SystemProgram":
				s.MissingSystemProgram = true
			default:
				if strings.HasPrefix(role, "ReceiverCPIAccount_") {
					s.MissingReceiverCPIAccount = true
				}
			}
		}
	}
	if len(seen) > 0 {
		s.MissingKeystoneReportRolesDedup = make([]string, 0, len(seen))
		for r := range seen {
			s.MissingKeystoneReportRolesDedup = append(s.MissingKeystoneReportRolesDedup, r)
		}
		sort.Strings(s.MissingKeystoneReportRolesDedup)
	}
	return s
}
