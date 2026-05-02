package broadcast

import "context"

type Noop struct{}

func (Noop) Publish(_ context.Context, _, _ string) error {
	return nil
}

func (Noop) Subscribe(ctx context.Context, _ string) (<-chan string, context.CancelFunc) {
	ch := make(chan string)
	subCtx, cancel := context.WithCancel(ctx)
	go func() {
		<-subCtx.Done()
		close(ch)
	}()
	return ch, cancel
}

func (Noop) Close() error {
	return nil
}
