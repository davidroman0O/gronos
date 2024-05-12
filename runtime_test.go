package gronos

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"
)

func testLifecycle(read chan envelope, done chan struct{}, shut chan struct{}) func(ctx context.Context, mailbox *Mailbox, courier *Courier, shutdown *Signal) error {
	return func(ctx context.Context, mailbox *Mailbox, courier *Courier, shutdown *Signal) error {
		for {
			select {
			case msg := <-mailbox.Read():
				slog.Info("lifecycle: runtime msg: ", slog.Any("msg", msg))
				read <- msg
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

func testLifecyclePanicked(read chan envelope, done chan struct{}, shut chan struct{}) func(ctx context.Context, mailbox *Mailbox, courier *Courier, shutdown *Signal) error {
	return func(ctx context.Context, mailbox *Mailbox, courier *Courier, shutdown *Signal) error {
		timer := time.NewTimer(time.Second * 2)
		for {
			select {
			case <-timer.C:
				panic("lifecycle panic: runtime panic")
			case msg := <-mailbox.Read():
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

	received := make(chan envelope)
	go func() {
		msg := <-runtime.courier.e
		received <- msg
		close(received)
	}()

	// we should wait for the context
	runtime.courier.Deliver(Envelope("destination", "test"))

	msg := <-received // should have receive
	if msg.Msg != "test" {
		t.Fatal("msg should be test")
	} else {
		slog.Info("msg received: ", slog.Any("msg", msg))
	}
	<-runtime.GracefulShutdown()
}

func TestRuntimeMailboxBasic(t *testing.T) {

	reading := make(chan envelope)

	runtime := newRuntime(
		"lifecycle",
		RuntimeWithRuntime(testLifecycle(reading, nil, nil)))

	<-runtime.
		Start().
		Await() // blocking until started

	slog.Info("runtime started")

	runtime.mailbox.post(Envelope("destination", "test"))

	// Since we have no clock in this test, we will push it ourselves
	runtime.mailbox.buffer.Tick() // process the mailbox

	msg := <-reading // should have receive

	if msg.Msg != "test" {
		t.Fatal("msg should be test")
	} else {
		slog.Info("msg received: ", slog.Any("msg", msg))
	}
	<-runtime.GracefulShutdown()
}
