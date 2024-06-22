package gronos

import (
	"context"
	"time"
)

// Cron is a function that runs a function periodically.
func Cron(duration time.Duration, fn func() error) RuntimeFunc {
	return func(ctx context.Context) error {

		shutdown, _ := UseShutdown(ctx)
		// courier, _ := UseCourier(ctx)

		ticker := time.NewTicker(duration)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-shutdown.Await():
				return nil
			case <-ticker.C:
				go func() {
					if err := fn(); err != nil {
						// courier.Transmit(err)
						select {
						case <-ctx.Done():
							return
						case <-shutdown.Await():
							return
						}
					}
				}()
			}
		}

		return nil
	}
}

// Timed is a function that runs a function periodically and waits for it to complete.
func Timed(duration time.Duration, fn func() error) RuntimeFunc {
	return func(ctx context.Context) error {

		shutdown, _ := UseShutdown(ctx)

		ticker := time.NewTicker(duration)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-shutdown.Await():
				return nil
			case <-ticker.C:
				if err := fn(); err != nil {
					// courier.Transmit(err)
					select {
					case <-ctx.Done():
						return nil
					case <-shutdown.Await():
						return nil
					}
				}
			}
		}

		return nil
	}
}
