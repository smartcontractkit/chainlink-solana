import { ConfirmedResult, Result } from '@chainlink/gauntlet-core'
import { logger } from '@chainlink/gauntlet-core/dist/utils'
import { TransactionResponse } from '../commands/types'

export const waitExecute = (execute: () => Promise<Result<TransactionResponse>>) =>
  async function (): Promise<ConfirmedResult<TransactionResponse>> {
    const result = await execute()
    logger.loading(`Waiting for tx confirmations...`)
    const txReceipts = await Promise.all(result.responses.map((r) => (r.tx ? r.tx.wait(r.tx.hash) : null)))
    logger.info(`${txReceipts.length} transactions confirmed`)
    const confirmedResult: ConfirmedResult<TransactionResponse> = {
      data: result.data,
      responses: txReceipts.map((tx, i) => ({ success: tx.success, ...result.responses[i] })),
    }
    return confirmedResult
  }
