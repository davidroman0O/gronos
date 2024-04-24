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

func TestSimple(t *testing.T) {

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

	signal, receiver := g.Run(ctx) // todo we should have a general context

	select {
	case <-signal.Await():
		slog.Info("signal")
	case err := <-receiver:
		slog.Info("error: ", err)
	case <-ctx.Done():
		slog.Info("ctx.Done()")
		clfn()
		g.Shutdown()
	}

	g.Wait()
}

func TestCron(t *testing.T) {

	g, err := New(
	// WithImmediateShutdown(),
	)
	if err != nil {
		t.Errorf("Error creating new context: %v", err)
	}

	_, clfn := g.Add(
		WithRuntime(Timed(1*time.Second, func() error {
			slog.Info("tick")
			return nil
		})),
		WithTimeout(time.Second*5),
		WithValue("testRuntime", "testRuntime"))

	ctx := context.Background()
	ctx, _ = context.WithTimeout(ctx, 7*time.Second)

	signal, receiver := g.Run(ctx) // todo we should have a general context

	select {
	case <-signal.Await():
		slog.Info("signal")
	case err := <-receiver:
		slog.Info("error: ", err)
	case <-ctx.Done():
		slog.Info("ctx.Done()")
		clfn()
		g.Shutdown()
	}

	g.Wait()
}

func testPing(pongID uint) RuntimeFunc {
	return func(ctx context.Context, mailbox *Mailbox, courier *Courier, shutdown *Signal) error {
		for {
			select {
			case msg := <-mailbox.Read():
				slog.Info("ping msg: ", slog.Any("msg", msg))
				courier.Deliver(Envelope{
					To:  pongID,
					Msg: "ping",
				})
			case <-ctx.Done():
				slog.Info("ping ctx.Done()")
				return nil
			case <-shutdown.Await():
				slog.Info("ping shutdown")
				courier.Notify(fmt.Errorf("shutdown"))
				return nil
			}
		}
	}
}

func testPong(pingID uint) RuntimeFunc {
	return func(ctx context.Context, mailbox *Mailbox, courier *Courier, shutdown *Signal) error {
		for {
			select {
			case msg := <-mailbox.Read():
				slog.Info("pong msg: ", slog.Any("msg", msg))
				courier.Deliver(Envelope{
					To:  pingID,
					Msg: "pong",
				})
			case <-ctx.Done():
				slog.Info("pong ctx.Done()")
				return nil
			case <-shutdown.Await():
				slog.Info("pong shutdown")
				courier.Notify(fmt.Errorf("shutdown"))
				return nil
			}
		}
	}
}

func TestCom(t *testing.T) {

	g, err := New(
	// WithImmediateShutdown(),
	)
	if err != nil {
		t.Errorf("Error creating new context: %v", err)
	}

	pingID := g.AddFuture()
	pongID := g.AddFuture()

	g.Push(pingID, WithRuntime(testPing(pongID)))
	g.Push(pongID, WithRuntime(testPong(pingID)))

	g.Send("ping", pongID)

	ctx := context.Background()
	ctx, cl := context.WithTimeout(ctx, 5*time.Second)

	signal, receiver := g.Run(ctx) // todo we should have a general context

	select {
	case <-signal.Await():
		slog.Info("signal ended")
		cl()
	case err := <-receiver:
		slog.Info("error: ", err)
	case <-ctx.Done():
		slog.Info("ctx.Done()")
		g.Shutdown()
		cl()
	}

	g.Wait()
}
