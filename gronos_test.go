package gronos

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"
)

// TODO Gronos talking to other Gronos

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

	g, err := New()
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

func TestTimed(t *testing.T) {

	g, err := New()
	if err != nil {
		t.Errorf("Error creating new context: %v", err)
	}

	_, clfn := g.Add(
		WithRuntime(
			// it should manage middlewares
			Timed(1*time.Second, func() error {
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

func testPing(pongID uint, counter *int) RuntimeFunc {
	return func(ctx context.Context, mailbox *Mailbox, courier *Courier, shutdown *Signal) error {
		for {
			select {
			case <-mailbox.Read():
				(*counter)++
				// slog.Info("ping msg: ", slog.Any("msg", msg))
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

func testPong(pingID uint, counter *int) RuntimeFunc {
	return func(ctx context.Context, mailbox *Mailbox, courier *Courier, shutdown *Signal) error {
		for {
			select {
			case <-mailbox.Read():
				(*counter)++
				// slog.Info("pong msg: ", slog.Any("msg", msg))
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

	g, err := New()
	if err != nil {
		t.Errorf("Error creating new context: %v", err)
	}

	counter := 0

	pingID := g.AddFuture()
	pongID := g.AddFuture()

	g.Push(pingID, WithRuntime(testPing(pongID, &counter)))
	g.Push(pongID, WithRuntime(testPong(pingID, &counter)))

	g.Send("ping", pongID)

	ctx := context.Background()
	ctx, cl := context.WithTimeout(ctx, 1*time.Second)
	defer cl()

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
	fmt.Println("counter", counter)
}

func testNamedWorker(name string) RuntimeFunc {
	return func(ctx context.Context, mailbox *Mailbox, courier *Courier, shutdown *Signal) error {

		fmt.Println(name + " test worker")
		pause, resume, ok := Paused(ctx)
		if !ok {
			return fmt.Errorf(name + " unsupported context type")
		}
		fmt.Println(name + " goroutine")

		go func() {
			for {
				select {
				// never forget ctx.Done()
				case <-ctx.Done():
					return
				// never forget shutdown signal
				case <-shutdown.Await():
					return
				// that's how you pause and resume
				case <-pause:
					fmt.Println(name + " )Worker paused")
					<-resume
					fmt.Println(name + " )Worker resumed")
				// default case is the actual work
				default:
					fmt.Println(name + " )Worker doing work...")
					time.Sleep(time.Second / 5)
				}
			}
		}()

		<-shutdown.Await()
		fmt.Println(name + " )shutdown runtime")

		return nil
	}
}

func TestPlay(t *testing.T) {
	g, err := New()
	if err != nil {
		t.Errorf("Error creating new context: %v", err)
	}

	id, _ := g.Add(WithRuntime(testNamedWorker("testPlayPause")))

	ctx := context.Background()
	ctx, cl := context.WithTimeout(ctx, 5*time.Second)
	defer cl()
	signal, receiver := g.Run(ctx) // todo we should have a general context

	go func() {
		time.Sleep(1 * time.Second)
		g.Pause(id)
		fmt.Println("paused")
		time.Sleep(2 * time.Second)
		g.Resume(id)
		fmt.Println("resume")
	}()

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
	time.Sleep(1 * time.Second)
}

func TestPhasingOut(t *testing.T) {
	g, err := New()
	if err != nil {
		t.Errorf("Error creating new context: %v", err)
	}

	id, _ := g.Add(WithRuntime(testNamedWorker("testPhasingOut")))

	ctx := context.Background()
	ctx, cl := context.WithTimeout(ctx, 5*time.Second)
	defer cl()
	signal, receiver := g.Run(ctx) // todo we should have a general context

	go func() {
		time.Sleep(1 * time.Second)
		g.PhasingOut(id)
		time.Sleep(2 * time.Second)
	}()

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
