package gronos

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// RuntimeFunc represents a function that runs a runtime.
type RuntimeFunc func(ctx context.Context, shutdown <-chan struct{}) <-chan error

// Config represents the configuration for the application.
type Config struct {
	// Add fields for your configuration here.
}

// WithXXX is an example constructor function that configures the Config struct.
func WithXXX(value string) func(*Config) error {
	return func(c *Config) error {
		// Configure the Config struct based on the provided value.
		return nil
	}
}

// Cron is a function that runs a function periodically.
func Cron(duration time.Duration, fn func() error) RuntimeFunc {
	return func(ctx context.Context, shutdown <-chan struct{}) <-chan error {
		errCh := make(chan error)

		go func() {
			defer close(errCh)

			ticker := time.NewTicker(duration)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-shutdown:
					return
				case <-ticker.C:
					go func() {
						if err := fn(); err != nil {
							select {
							case errCh <- err:
							case <-ctx.Done():
								return
							case <-shutdown:
								return
							}
						}
					}()
				}
			}
		}()

		return errCh
	}
}

// Timed is a function that runs a function periodically and waits for it to complete.
func Timed(duration time.Duration, fn func() error) RuntimeFunc {
	return func(ctx context.Context, shutdown <-chan struct{}) <-chan error {
		errCh := make(chan error)

		go func() {
			defer close(errCh)

			ticker := time.NewTicker(duration)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-shutdown:
					return
				case <-ticker.C:
					if err := fn(); err != nil {
						select {
						case errCh <- err:
						case <-ctx.Done():
							return
						case <-shutdown:
							return
						}
					}
				}
			}
		}()

		return errCh
	}
}

// Run is the bootstrapping function that manages the lifecycle of the application.
func Run(cfg *Config, runtimes ...RuntimeFunc) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdown := make(chan struct{})

	errCh := make(chan error, 1) // Buffered channel to prevent panic on multiple ctrl+c signals
	var wg sync.WaitGroup

	for _, runtime := range runtimes {
		wg.Add(1)
		go func(r RuntimeFunc) {
			defer wg.Done()
			for err := range r(ctx, shutdown) {
				errCh <- err
			}
		}(runtime)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		close(shutdown)
		cancel()
		close(errCh)
	}()

	wg.Wait()

	if len(errCh) > 0 {
		return <-errCh
	}

	return nil
}
