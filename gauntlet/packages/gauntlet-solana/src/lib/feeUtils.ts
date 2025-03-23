import { PublicKey, BlockResponse } from '@solana/web3.js'

export interface BlockData {
  fees: number[]
  prices: number[]
}

const COMPUTE_BUDGET_PROGRAM_ID = new PublicKey('ComputeBudget111111111111111111111111111111')

// Instructions are serialized in order in rust
// https://github.com/solana-labs/solana/blob/60858d043ca612334de300805d93ea3014e8ab37/sdk/src/compute_budget.rs#L25
enum ComputeBudgetInstruction {
  SetComputeUnitLimit = 0x02, // Previously 0x02, now 0x03 in latest versions
  SetComputeUnitPrice = 0x03, // Previously 0x02
}
export const parseBlock = (block: BlockResponse): BlockData => {
  const blockData: BlockData = { fees: [], prices: [] }
  for (const tx of block.transactions) {
    const meta = tx.meta
    let computeUnitPrice: number
    for (const ins of tx.transaction.message.instructions) {
      const programId = tx.transaction.message.accountKeys[ins.programIdIndex]

      // Check if instruction is from Compute Budget Program
      if (programId.equals(COMPUTE_BUDGET_PROGRAM_ID)) {
        const parsedPrice = parseComputeUnitPrice(Buffer.from(ins.data))
        if (parsedPrice !== null) {
          computeUnitPrice = parsedPrice
          break // Only one SetComputeUnitPrice instruction per transaction
        }
      }
    }

    if (meta && computeUnitPrice) {
      blockData.fees = [...blockData.fees, meta.fee]
      blockData.prices = [...blockData.prices, computeUnitPrice]
    }
  }

  return blockData
}

export const parseComputeBudgetInstruction = (
  instructionType: ComputeBudgetInstruction,
  data: Buffer,
): number | null => {
  const expectedLength = instructionType === ComputeBudgetInstruction.SetComputeUnitPrice ? 9 : 5 // 1-byte discriminator + u64 (8 bytes) or u32 (4 bytes)

  if (data.length !== expectedLength) {
    console.error(
      `Invalid length for ${ComputeBudgetInstruction[instructionType]}: expected ${expectedLength}, got ${data.length}`,
    )
    return null
  }

  // Validate instruction identifier
  if (data[0] !== instructionType) {
    console.error(`Not a ${ComputeBudgetInstruction[instructionType]} instruction: got ${data[0]}`)
    return null
  }

  // Decode the value (Little Endian)
  return instructionType === ComputeBudgetInstruction.SetComputeUnitPrice
    ? Number(data.readBigUInt64LE(1)) // u64 for SetComputeUnitPrice
    : data.readUInt32LE(1) // u32 for SetComputeUnitLimit
}

export const parseComputeUnitPrice = (data: Buffer): number | null => {
  return parseComputeBudgetInstruction(ComputeBudgetInstruction.SetComputeUnitPrice, data)
}

export const parseComputeUnitLimit = (data: Buffer): number | null => {
  return parseComputeBudgetInstruction(ComputeBudgetInstruction.SetComputeUnitLimit, data)
}
