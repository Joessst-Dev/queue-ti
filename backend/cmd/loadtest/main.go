package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	pb "github.com/Joessst-Dev/queue-ti/pb"
)

// bearerToken implements credentials.PerRPCCredentials so that every gRPC call
// carries an "authorization: Bearer <token>" metadata header.
type bearerToken struct {
	token string
}

func (b bearerToken) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + b.token,
	}, nil
}

func (b bearerToken) RequireTransportSecurity() bool {
	return false
}

// stats accumulates raw latency samples and error counts for one operation kind.
type stats struct {
	mu       sync.Mutex
	samples  []time.Duration
	errors   int64
	firstErr string
}

// record saves a latency sample on success or increments the error counter on
// failure. It returns true the first time an error is seen so the caller can
// log it immediately.
func (s *stats) record(d time.Duration, err error) (firstError bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.errors++
		if s.firstErr == "" {
			s.firstErr = err.Error()
			return true
		}
		return false
	}
	s.samples = append(s.samples, d)
	return false
}

// percentile returns the p-th percentile (0–100) from a sorted duration slice.
// It assumes samples is already sorted and non-empty.
func percentile(sorted []time.Duration, p float64) time.Duration {
	idx := int(float64(len(sorted)-1) * p / 100.0)
	return sorted[idx]
}

// summarise prints the final statistics section for one operation kind.
func summarise(w *os.File, label string, s *stats, elapsed time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	total := int64(len(s.samples))
	throughput := float64(total) / elapsed.Seconds()

	fmt.Fprintf(w, "%s\n", label)
	fmt.Fprintf(w, "  Total:      %s ops\n", formatInt(total))
	fmt.Fprintf(w, "  Throughput: %s ops/s\n", formatFloat(throughput))

	if total == 0 {
		fmt.Fprintf(w, "  p50:        n/a\n")
		fmt.Fprintf(w, "  p95:        n/a\n")
		fmt.Fprintf(w, "  p99:        n/a\n")
	} else {
		sorted := make([]time.Duration, len(s.samples))
		copy(sorted, s.samples)
		slices.Sort(sorted)

		fmt.Fprintf(w, "  p50:        %s\n", formatDuration(percentile(sorted, 50)))
		fmt.Fprintf(w, "  p95:        %s\n", formatDuration(percentile(sorted, 95)))
		fmt.Fprintf(w, "  p99:        %s\n", formatDuration(percentile(sorted, 99)))
	}

	fmt.Fprintf(w, "  Errors:     %d\n", s.errors)
	if s.errors > 0 && s.firstErr != "" {
		fmt.Fprintf(w, "  First error: %s\n", s.firstErr)
	}
}

// formatInt formats an integer with thousands separators.
func formatInt(n int64) string {
	s := fmt.Sprintf("%d", n)
	result := make([]byte, 0, len(s)+len(s)/3)
	for i, c := range s {
		pos := len(s) - i
		if i > 0 && pos%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}

// formatFloat formats a float with thousands separators and one decimal place.
func formatFloat(f float64) string {
	whole := int64(f)
	frac := int(f*10) % 10
	return fmt.Sprintf("%s.%d", formatInt(whole), frac)
}

// formatDuration formats a duration as a human-readable latency string (e.g. "2.1ms").
func formatDuration(d time.Duration) string {
	switch {
	case d >= time.Second:
		return fmt.Sprintf("%.1fs", d.Seconds())
	case d >= time.Millisecond:
		return fmt.Sprintf("%.1fms", float64(d)/float64(time.Millisecond))
	default:
		return fmt.Sprintf("%.1fµs", float64(d)/float64(time.Microsecond))
	}
}

func main() {
	addr := flag.String("addr", "localhost:50051", "gRPC server address")
	duration := flag.Duration("duration", 30*time.Second, "test duration")
	producers := flag.Int("producers", 4, "number of concurrent producer goroutines")
	consumers := flag.Int("consumers", 4, "number of concurrent consumer goroutines")
	topic := flag.String("topic", "loadtest", "topic name to use")
	msgSize := flag.Int("msg-size", 256, "payload size in bytes")
	token := flag.String("token", "", "optional Bearer JWT token for auth")
	flag.Parse()

	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	if *token != "" {
		dialOpts = append(dialOpts, grpc.WithPerRPCCredentials(bearerToken{token: *token}))
	}

	conn, err := grpc.NewClient(*addr, dialOpts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to %s: %v\n", *addr, err)
		os.Exit(1)
	}
	defer conn.Close()

	client := pb.NewQueueServiceClient(conn)
	payload := make([]byte, *msgSize)

	ctx, cancel := context.WithCancel(context.Background())

	// Honour OS interrupt signals in addition to the timed expiry.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	enqueueStats := &stats{}
	dequeueAckStats := &stats{}

	var wg sync.WaitGroup

	// Progress reporter — prints a brief line every 5 seconds until context done.
	wg.Go(func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		start := time.Now()
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-ticker.C:
				elapsed := t.Sub(start).Truncate(time.Second)
				enqueueStats.mu.Lock()
				eq := int64(len(enqueueStats.samples))
				eqErr := enqueueStats.errors
				enqueueStats.mu.Unlock()
				dequeueAckStats.mu.Lock()
				dq := int64(len(dequeueAckStats.samples))
				dqErr := dequeueAckStats.errors
				dequeueAckStats.mu.Unlock()
				fmt.Fprintf(os.Stderr, "[%s] enqueue: %s (err: %s) | dequeue+ack: %s (err: %s)\n",
					elapsed,
					formatInt(eq), formatInt(eqErr),
					formatInt(dq), formatInt(dqErr),
				)
			}
		}
	})

	// Producer goroutines.
	for range *producers {
		wg.Go(func() {
			for {
				if ctx.Err() != nil {
					return
				}
				start := time.Now()
				_, err := client.Enqueue(ctx, &pb.EnqueueRequest{
					Topic:   *topic,
					Payload: payload,
				})
				elapsed := time.Since(start)
				if ctx.Err() != nil {
					// Ignore errors caused by context cancellation at shutdown.
					return
				}
				if first := enqueueStats.record(elapsed, err); first {
					fmt.Fprintf(os.Stderr, "enqueue error (first): %v\n", err)
				}
				if err != nil {
					time.Sleep(100 * time.Millisecond)
				}
			}
		})
	}

	// Consumer goroutines.
	for range *consumers {
		wg.Go(func() {
			for {
				if ctx.Err() != nil {
					return
				}

				start := time.Now()
				resp, err := client.Dequeue(ctx, &pb.DequeueRequest{Topic: *topic})
				if ctx.Err() != nil {
					return
				}

				if err != nil {
					if status.Code(err) == codes.NotFound {
						// Queue is empty — back off briefly before retrying.
						time.Sleep(10 * time.Millisecond)
						continue
					}
					if first := dequeueAckStats.record(0, err); first {
						fmt.Fprintf(os.Stderr, "dequeue error (first): %v\n", err)
					}
					time.Sleep(100 * time.Millisecond)
					continue
				}

				_, ackErr := client.Ack(ctx, &pb.AckRequest{Id: resp.Id})
				elapsed := time.Since(start)
				if ctx.Err() != nil {
					return
				}
				dequeueAckStats.record(elapsed, ackErr)
			}
		})
	}

	// Let the test run for the requested duration, then cancel.
	timer := time.NewTimer(*duration)
	select {
	case <-timer.C:
		cancel()
	case <-ctx.Done():
		timer.Stop()
	}

	wg.Wait()

	fmt.Fprintf(os.Stdout, "\n=== Load Test Results (%s, %d producers, %d consumers) ===\n\n",
		*duration,
		*producers,
		*consumers,
	)
	summarise(os.Stdout, "Enqueue", enqueueStats, *duration)
	fmt.Fprintln(os.Stdout)
	summarise(os.Stdout, "Dequeue+Ack", dequeueAckStats, *duration)
	fmt.Fprintln(os.Stdout)
}
