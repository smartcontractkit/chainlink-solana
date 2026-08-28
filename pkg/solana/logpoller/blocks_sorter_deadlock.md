# Log Poller deadlock: `blocksSorter` never closes `outBlocks`

## Summary

When a block fetch is aborted, the log poller's block sorter can shut down **without
closing its output channel**, leaving the consumer in `processBlocksRange` blocked
forever on a receive that will never complete. The backfill/replay stalls with no
timeout and no error.

## The pipeline

`BackfillForAddresses` (loader.go) wires the whole thing together:

1. `getSlotsToFetch` resolves the `[fromSlot, toSlot]` range down to the subset of
   slots that actually contain transactions for the watched addresses.
2. `scheduleBlocksFetching` spawns one `getBlockJob` per slot and returns the
   `unorderedBlocks` channel. Each job pushes its fetched block into that channel.
   When **all** jobs are done, `scheduleBlocksFetching` calls `close(blocks)`.
3. `newBlocksSorter(unorderedBlocks, …)` takes that channel as its `inBlocks` and
   re-emits the blocks **in slot order** on `outBlocks`, which is what
   `processBlocksRange` consumes.

So `unorderedBlocks` is the hand-off: the sorter reads it, and its closure is the
signal that no more blocks are coming.

## Where it breaks

### 1. `inBlocks` closing is the "we're done" signal

In `readBlocks` (blocks_sorter.go), when `scheduleBlocksFetching` closes the channel:

```go
case block, ok := <-p.inBlocks:
    if !ok {
        close(p.receivedNewBlock) // trigger last flush of ready blocks
        return
    }
```

Closing `receivedNewBlock` is what tells `writeOrderedBlocks` to do its final flush
and wind down.

### 2. `writeOrderedBlocks` only closes `outBlocks` if the queue is empty

```go
case _, ok := <-p.receivedNewBlock:
    p.flushReadyBlocks(ctx)
    if !ok {                          // receivedNewBlock was closed → final pass
        p.mu.Lock()
        if p.queue.Len() == 0 {       // <-- gate
            close(p.outBlocks)
        }
        p.mu.Unlock()
        return                        // exits WITHOUT closing outBlocks
    }
```

The intent is "only signal completion once every expected block has been emitted."
The queue still holds every slot that hasn't been emitted yet — so if anything is
left over, `close(p.outBlocks)` is **skipped**, and the goroutine returns. Nothing
else ever closes `outBlocks`.

### 3. `flushReadyBlocks` returns early on an aborted block, leaving the queue non-empty

`getBlockJob.Abort` doesn't skip the send — it pushes a block with `Aborted = true`
into `unorderedBlocks`, so the aborted block lands in the sorter's `readyBlocks`.

```go
for {
    block := p.readNextReadyBlock()   // pops queue.Front() and REMOVES it
    if block == nil {
        return
    }
    if block.Aborted {
        return                        // <-- BUG: bails, rest of queue never flushed
    }
    ...
}
```

`readNextReadyBlock` has already removed the head element before the `Aborted` check.
So when the head block is aborted, `flushReadyBlocks` pops it and returns immediately,
**without draining the blocks still queued behind it.**

## The deadlock, end to end

- `receivedNewBlock` has capacity 1 and `readBlocks` only does a best-effort
  non-blocking send, so notifications **coalesce** — many queued blocks can be
  represented by a single wakeup.
- On the final wakeup (`ok == false`), `flushReadyBlocks` pops an aborted head block
  and returns early with the queue still non-empty.
- `writeOrderedBlocks` sees `queue.Len() != 0`, skips `close(p.outBlocks)`, and returns.
- The consumer in `processBlocksRange` (`case block, ok := <-blocks:`) blocks forever
  on a channel that is never written to or closed again — no timeout, no error.

## Fix

Skip the aborted block and keep draining instead of returning:

```go
for {
    block := p.readNextReadyBlock()
    if block == nil {
        return
    }
    if block.Aborted {
        continue // drop it, keep flushing the rest
    }
    select {
    case p.outBlocks <- *block:
    case <-ctx.Done():
        return
    }
}
```

With the queue fully drained on every flush, the final pass always sees
`queue.Len() == 0` and `close(p.outBlocks)` fires, so the consumer terminates cleanly.
