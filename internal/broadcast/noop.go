package broadcast

import "context"

type Noop struct{}

func (Noop) Publish(_ context.Context, _, _ string) error {
	return nil
}

func (Noop) Subscribe(_ context.Context, _ string) (<-chan string, context.CancelFunc) {
	ch := make(chan string)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, cancel
}

func (Noop) Close() error {
	return nil
}
