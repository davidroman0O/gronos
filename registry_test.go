package gronos

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/davidroman0O/gronos/clock"
)

func TestRuntimeBasic(t *testing.T) {
	registry := NewRuntimeRegistry()

	ck := clock.New(clock.WithInterval(time.Millisecond * 100))

	// continous execution
	ck.Add(registry, clock.ManagedTimeline)
	defer ck.Stop()

	// nothing will run without that
	ck.Start()

	aID, _ := registry.Add(
		"a",
		RuntimeWithRuntime(
			func(ctx context.Context) error {
				mailbox, _ := UseMailbox(ctx)
				shutdown, _ := UseShutdown(ctx)
				for {
					select {
					case msg := <-mailbox:
						slog.Info("a runtime msg: ", slog.Any("msg", msg))
					case <-ctx.Done():
						slog.Info("a runtime ctx.Done()")
						return nil
					case <-shutdown.Await():
						slog.Info("a runtime shutdown")
						return nil
					}
				}
			},
		),
	)

	bID, _ := registry.Add(
		"b",
		RuntimeWithRuntime(
			func(ctx context.Context) error {
				mailbox, _ := UseMailbox(ctx)
				shutdown, _ := UseShutdown(ctx)
				for {
					select {
					case msg := <-mailbox:
						slog.Info("b runtime msg: ", slog.Any("msg", msg))
					case <-ctx.Done():
						slog.Info("b runtime ctx.Done()")
						return nil
					case <-shutdown.Await():
						slog.Info("b runtime shutdown")
						return nil
					}
				}
			},
		),
	)

	runtimeA, ok := registry.Get(aID)
	if !ok {
		t.Error("runtime A not found")
	}

	runtimeB, ok := registry.Get(bID)
	if !ok {
		t.Error("runtime B not found")
	}

	ctx := context.Background()
	ctx, cl := context.WithTimeout(ctx, 1*time.Second)
	defer cl()

	<-ctx.Done()
	<-runtimeA.GracefulShutdown()

	<-registry.WhenID(aID, stateStopped)

	ctx = context.Background()
	ctx, cl = context.WithTimeout(ctx, 1*time.Second)
	defer cl()
	<-ctx.Done()
	<-runtimeB.GracefulShutdown()
	<-registry.WhenID(bID, stateStopped)
}

func TestRegistryPanic(t *testing.T) {

	registry := NewRuntimeRegistry()

	ck := clock.New(clock.WithInterval(time.Millisecond * 100))

	// continous execution
	ck.Add(registry, clock.ManagedTimeline)
	defer ck.Stop()

	aID, _ := registry.Add(
		"a",
		RuntimeWithTimeout(time.Second*1),
		RuntimeWithRuntime(
			func(ctx context.Context) error {
				mailbox, _ := UseMailbox(ctx)
				shutdown, _ := UseShutdown(ctx)
				for {
					select {
					case msg := <-mailbox:
						slog.Info("a runtime msg: ", slog.Any("msg", msg))
					case <-ctx.Done():
						panic("aaaaaah")
						slog.Info("a runtime ctx.Done()")
						return nil
					case <-shutdown.Await():
						slog.Info("a runtime shutdown")
						return nil
					}
				}
			},
		),
	)

	// nothing will run without that
	ck.Start()
	<-registry.WhenID(aID, stateStarted)
	fmt.Println("started")
	<-registry.WhenID(aID, statePanicked)
	runtimeA, ok := registry.Get(aID)
	if !ok {
		t.Error("runtime A not found")
	}

	fmt.Println("paniked", runtimeA.perr)

}
