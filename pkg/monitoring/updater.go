package monitoring

import "context"

// TODO (dru) move this to relay

type Updater interface {
	// Run should be executed as a goroutine otherwise it will block.
	Run(context.Context)
	// You should never close the channel returned by Updates()!
	// You should always read from the channel returned by Updates() in a
	// select statement with the same context you passed to Run()
	Updates() <-chan interface{}
}
