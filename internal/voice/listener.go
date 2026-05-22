package voice

import "context"

type Listener interface {
	Listen(ctx context.Context) (<-chan string, error)
}
