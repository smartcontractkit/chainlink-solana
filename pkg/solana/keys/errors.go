package keys

import "fmt"

type KeyNotFoundError struct {
	ID      string
	KeyType string
}

func (e KeyNotFoundError) Error() string {
	return fmt.Sprintf("unable to find %s key with id %s", e.KeyType, e.ID)
}
