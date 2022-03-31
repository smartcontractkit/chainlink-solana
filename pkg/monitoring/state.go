package monitoring

// TODO (dru) move this to relay

type State interface {
	Get(key []byte) (value []byte, err error)
	Set(key, value []byte) (err error)
}

type StateFactory interface {
	NewState(namespace string) (State, error)
}
