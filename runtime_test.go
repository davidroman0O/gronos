package gronos

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"
)

func testLifecycle(read chan message, done chan struct{}, shut chan struct{}) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		mailbox, _ := UseMailbox(ctx)
		shutdown, _ := UseShutdown(ctx)
		for {
			select {
			case msgs := <-mailbox:
				slog.Info("lifecycle: runtime msg: ", slog.Any("msg", msgs))
				for i := 0; i < len(msgs); i++ {
					read <- msgs[i]
				}
				close(read)
			case <-ctx.Done():
				slog.Info("lifecycle: runtime ctx.Done()")
				close(done)
				return nil
			case <-shutdown.Await():
				slog.Info("lifecycle: runtime shutdown")
				close(shut)
				return nil
			}
		}
		return fmt.Errorf("lifecycle runtime error")
	}
}

// Test is supposed to test the basic lifecycle with a timeout
func TestRuntimeLifecycleBasicTimeout(t *testing.T) {

	durationTimeout := time.Second * 1

	waitingContext := make(chan struct{})

	runtime := newRuntime(
		"lifecycle",
		RuntimeWithRuntime(testLifecycle(nil, waitingContext, nil)), // on purpose, i rather panic
		RuntimeWithTimeout(durationTimeout))

	<-runtime.Start().Await() // blocking until started
	slog.Info("runtime started")
	now := time.Now()

	// we should wait for the context
	<-waitingContext

	// add 100ms to be nice cause cpu might be busy
	if time.Since(now) > durationTimeout+time.Millisecond*100 {
		t.Fatal("timeout should have been triggered")
	}
}

func TestRuntimeLifecycleBasicShutdown(t *testing.T) {

	waitingShutdown := make(chan struct{})

	runtime := newRuntime(
		"lifecycle",
		RuntimeWithRuntime(testLifecycle(nil, nil, waitingShutdown)))

	<-runtime.Start().Await() // blocking until started
	slog.Info("runtime started")
	now := time.Now()

	go func() {
		timer := time.NewTimer(time.Second * 2)
		<-timer.C
		runtime.GracefulShutdown()
	}()

	// we should wait for the context
	<-waitingShutdown
	if time.Since(now) > (time.Second*2)+time.Millisecond*100 {
		t.Fatal("shutdown should have been triggered")
	}
}

func testLifecyclePanicked(read chan message, done chan struct{}, shut chan struct{}) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		timer := time.NewTimer(time.Second * 2)
		mailbox, _ := UseMailbox(ctx)
		shutdown, _ := UseShutdown(ctx)
		for {
			select {
			case <-timer.C:
				panic("lifecycle panic: runtime panic")
			case msg := <-mailbox:
				slog.Info("lifecycle panic: runtime msg: ", slog.Any("msg", msg))
				close(read)
			case <-ctx.Done():
				slog.Info("lifecycle panic: runtime ctx.Done()")
				close(done)
				return nil
			case <-shutdown.Await():
				slog.Info("lifecycle panic: runtime shutdown")
				close(shut)
				return nil
			}
		}
		return fmt.Errorf("lifecycle panic: runtime error")
	}
}

func TestRuntimeLifecyclePanic(t *testing.T) {

	runtime := newRuntime(
		"lifecycle",
		RuntimeWithRuntime(testLifecyclePanicked(nil, nil, nil)))

	listener := make(chan struct{})

	<-runtime.Start(
		WithPanic(func(recover interface{}) {
			slog.Info("panic: ", slog.Any("recover", recover))
			close(listener)
		}),
	).Await() // blocking until started
	slog.Info("runtime started")
	now := time.Now()

	// we should wait for the context
	<-listener
	if time.Since(now) > (time.Second*2)+time.Millisecond*100 {
		t.Fatal("shutdown should have been triggered")
	}
}

// Supposed to send a message to somewhere
func TestRuntimeCourierBasic(t *testing.T) {

	runtime := newRuntime(
		"lifecycle",
		RuntimeWithRuntime(testLifecycle(nil, nil, nil)))

	<-runtime.
		Start().
		Await() // blocking until started

	slog.Info("runtime started")

	received := make(chan []message)
	go func() {
		mailbox, ok := UseMailbox(runtime.ctx)
		if !ok {
			t.Fatal("mailbox not found")
		}
		msgs := <-mailbox
		fmt.Println("received ", msgs)
		received <- msgs
		close(received)
	}()

	devlieryMsg, _ := NewMessage("test")

	courier, _ := UseCourier(runtime.ctx) // we should wait for the context
	courier.Deliver(*devlieryMsg)

	receivedMessage := false
	for _, msg := range <-received {
		if msg.Payload != "test" {
			t.Fatal("msg should be test")
		} else {
			receivedMessage = true
			slog.Info("msg received: ", slog.Any("msg", msg))
		}
	}
	<-runtime.GracefulShutdown()
	if !receivedMessage {
		t.Fatal("should have received a message")
	}
}

func TestRuntimeMailboxBasic(t *testing.T) {

	reading := make(chan message)

	runtime := newRuntime(
		"lifecycle",
		RuntimeWithRuntime(testLifecycle(reading, nil, nil)))

	<-runtime.
		Start().
		Await() // blocking until started

	slog.Info("runtime started")

	// devlieryMsg, _ := NewMessage("test")
	// runtime.mailbox.Publish(*devlieryMsg)

	// Since we have no clock in this test, we will push it ourselves
	// runtime.mailbox.buffer.Tick() // process the mailbox

	msg := <-reading // should have receive

	if msg.Payload != "test" {
		t.Fatal("msg should be test")
	} else {
		slog.Info("msg received: ", slog.Any("msg", msg))
	}
	<-runtime.GracefulShutdown()
}
