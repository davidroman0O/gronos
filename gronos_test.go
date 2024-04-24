package gronos

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"
)

func testRuntime(ctx context.Context, mailbox *Mailbox, courier *Courier, shutdown *Signal) error {
	slog.Info("runtime ")
	fmt.Println("runtime  value", ctx.Value("testRuntime"))

	courier.Deliver(Envelope{
		To:  0,
		Msg: "hello myself",
	})

	for {
		select {
		case msg := <-mailbox.Read():
			slog.Info("runtime msg: ", slog.Any("msg", msg))
		case <-ctx.Done():
			slog.Info("runtime ctx.Done()")
			return nil
		case <-shutdown.Await():
			slog.Info("runtime shutdown")
			courier.Notify(fmt.Errorf("shutdown"))
			return nil
		}
	}

	return fmt.Errorf("runtime error")
}

func TestSimpleStack(t *testing.T) {

	g, err := New(
	// WithImmediateShutdown(),
	)
	if err != nil {
		t.Errorf("Error creating new context: %v", err)
	}

	_, clfn := g.Add(
		WithRuntime(testRuntime),
		WithTimeout(time.Second*5),
		WithValue("testRuntime", "testRuntime"))

	ctx := context.Background()
	ctx, _ = context.WithTimeout(ctx, 7*time.Second)

	lifeline, receiver := g.Run() // todo we should have a general context

	select {
	case <-lifeline.Await():
		slog.Info("lifeline")
	case err := <-receiver:
		slog.Info("error: ", err)
	case <-ctx.Done():
		slog.Info("ctx.Done()")
		clfn()
		g.Shutdown()
	}

	g.Wait()
}
