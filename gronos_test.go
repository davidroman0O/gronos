package gronos

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/k0kubun/pp/v3"
)

// TODO Gronos talking to other Gronos

func testRuntime(ctx context.Context, mailbox *Mailbox, courier *Courier, shutdown *Signal) error {
	slog.Info("runtime ")
	fmt.Println("runtime  value", ctx.Value("testRuntime"))

	// courier.Deliver(Envelope{
	// 	To:  0,
	// 	Msg: "hello myself",
	// })
	courier.Deliver(Envelope("simple", "hello myself"))

	for {
		select {
		case msg := <-mailbox.Read():
			slog.Info("runtime msg: ", slog.Any("msg", msg))
		case <-ctx.Done():
			slog.Info("runtime ctx.Done()")
			return nil
		case <-shutdown.Await():
			slog.Info("runtime shutdown")
			courier.Transmit(fmt.Errorf("shutdown"))
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
		"simple",
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
		"ticker",
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

func testPing(counter *int) RuntimeFunc {
	return func(ctx context.Context, mailbox *Mailbox, courier *Courier, shutdown *Signal) error {
		for {
			select {
			case <-mailbox.Read():
				(*counter)++
				// slog.Info("ping msg: ", slog.Any("msg", msg))
				// courier.Deliver(Envelope{
				// 	To:  pongID,
				// 	Msg: "ping",
				// })
				courier.Deliver(Envelope("pong", "ping"))
			case <-ctx.Done():
				slog.Info("ping ctx.Done()")
				return nil
			case <-shutdown.Await():
				slog.Info("ping shutdown")
				courier.Transmit(fmt.Errorf("shutdown"))
				return nil
			}
		}
	}
}

func testPong(counter *int) RuntimeFunc {
	return func(ctx context.Context, mailbox *Mailbox, courier *Courier, shutdown *Signal) error {
		for {
			select {
			case <-mailbox.Read():
				(*counter)++
				// slog.Info("pong msg: ", slog.Any("msg", msg))
				// courier.Deliver(Envelope{
				// 	To:  pingID,
				// 	Msg: "pong",
				// })
				courier.Deliver(Envelope("ping", "pong"))
			case <-ctx.Done():
				slog.Info("pong ctx.Done()")
				return nil
			case <-shutdown.Await():
				slog.Info("pong shutdown")
				courier.Transmit(fmt.Errorf("shutdown"))
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

	// pingID := g.AddFuture()
	// pongID := g.AddFuture()

	g.Add("ping", WithRuntime(testPing(&counter)))
	g.Add("pong", WithRuntime(testPong(&counter)))

	if err = g.Named("pong", "ping"); err != nil {
		t.Errorf("Error naming pong: %v", err)
	}

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
			pp.Println(ctx)
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

	id, _ := g.Add("worker", WithRuntime(testNamedWorker("testPlayPause")))

	ctx := context.Background()
	ctx, cl := context.WithTimeout(ctx, 5*time.Second)
	defer cl()
	signal, receiver := g.Run(ctx) // todo we should have a general context

	go func() {
		time.Sleep(1 * time.Second)
		g.DirectPause(id)
		fmt.Println("paused")
		time.Sleep(2 * time.Second)
		g.DirectResume(id)
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

	id, _ := g.Add("worker", WithRuntime(testNamedWorker("testPhasingOut")))

	ctx := context.Background()
	ctx, cl := context.WithTimeout(ctx, 5*time.Second)
	defer cl()
	signal, receiver := g.Run(ctx) // todo we should have a general context

	go func() {
		time.Sleep(1 * time.Second)
		g.DirectPhasingOut(id)
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

func TestGronosToGronos(t *testing.T) {

}
