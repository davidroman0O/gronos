package gronos

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func testRuntimeReaderStream(ctx context.Context) error {
	fmt.Println("testRuntimeReaderStream some")

	shutdown, ok := UseShutdown(ctx)
	if !ok {
		return fmt.Errorf("no shutdown")
	}

	incomingMsgs, ok := UseMailbox(ctx)
	if ok {
		go func() {
			for {
				select {
				case msgs := <-incomingMsgs:
					// for _, msg := range msgs {
					// 	fmt.Println("testRuntimeReaderStream message received", msg)
					// }
					fmt.Println(len(msgs))
				case <-shutdown.Await():
					fmt.Println("testRuntimeReaderStream mailbox shutdown")
					return
				}
			}
		}()
	}

	fmt.Println("testRuntimeReaderStream waiting")
	<-shutdown.Await()
	fmt.Println("testRuntimeReaderStream shutdown")

	return nil
}

// Basic one runtime that we push messages constantly
func TestGronosBasics(t *testing.T) {

	g := New(
		WithCourierInterval(100*time.Microsecond),
		WithRouterInterval(100*time.Microsecond),
	)

	g.Add("testA", RuntimeWithRuntime(testRuntimeReaderStream))

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				g.Push(NewMessage(context.Background(), NewMetadata(WithTo("testA")), "hello"))
			}
		}
	}()

	<-ctx.Done()

	g.Close()
}

func testRuntimeB(ctx context.Context) error {
	fmt.Println("testRuntimeB some")

	courier, ok := UseCourier(ctx)
	if !ok {
		return fmt.Errorf("no courier")
	}

	shutdown, ok := UseShutdown(ctx)
	if !ok {
		return fmt.Errorf("no shutdown")
	}

	tickerrrr := time.NewTicker(10 * time.Millisecond)

	incomingMsgs, ok := UseMailbox(ctx)
	if ok {
		go func() {
			for {
				select {
				case <-tickerrrr.C:
					courier(
						NewMessage(context.Background(), NewMetadata(WithTo("testA")), "hello"),
					)
				case msgs := <-incomingMsgs:
					for _, msg := range msgs {
						fmt.Println("testRuntimeA message received", msg)
					}
				case <-shutdown.Await():
					fmt.Println("testRuntimeA mailbox shutdown")
					return
				}
			}
		}()
	}

	fmt.Println("testRuntimeB waiting")
	<-shutdown.Await()
	fmt.Println("testRuntimeB shutdown")

	return nil
}

// TODO: make a test with concurrency so we can see if we actually crash or not with the courier
// TODO: make a performance test
