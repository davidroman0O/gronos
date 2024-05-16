package gronos

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

// / The whole goal of this test is now to refine the whole communication between the router -> registry -> runtime
// / For now, it is still the old system, and require to evolve
func TestRouterBasic(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Second*4)
	defer cancel()

	router := newRouter() // when starting, the registry will also start
	router.clock.Start()  // which starts the RuntimeRegistry too

	timer := time.NewTicker(time.Millisecond * 500)
	id, _ := router.Add("simple",
		RuntimeWithRuntime(
			func(ctx context.Context, mailbox *Mailbox, courrier *Courier, shutdown *Signal) error {
				slog.Info("runtime started")
				for {
					select {
					case <-timer.C:
						slog.Info("runtime tick")
						continue
					case msg := <-mailbox.Read():
						slog.Info("runtime: runtime msg: ", slog.Any("msg", msg))
						cancel()
						continue
					case <-ctx.Done():
						slog.Info("runtime: runtime ctx.Done()")
						continue
					case <-shutdown.Await():
						slog.Info("runtime: runtime shutdown")
						continue
					}
				}
			}))

	msg, _ := NewMessage("test")

	// send message to the mailbox of the router (pre-process)
	if err := router.Tell(id, *msg); err != nil {
		t.Fatalf("Error telling: %v\n", err)
	}

	router.mailbox.Tick() // manually trigger the mailbox to process the message
	router.Tick()         // manually trigger the router to pre-process the analysis

	router.registry.mailbox.Tick() // manually trigger the registry mailbox to process the message
	router.registry.Tick()         // manually trigger the registry to process the message

	<-ctx.Done()
	router.clock.Stop()
	timer.Stop()
}

// func TestRouterWhenStarted(t *testing.T) {
// 	router := newRouter()

// 	runtime := newRuntime(
// 		"simple",
// 		RuntimeWithRuntime(testRuntime),
// 		RuntimeWithTimeout(time.Second*5),
// 		RuntimeWithValue("testRuntime", "testRuntime"))

// 	// router.Add(*runtime)

// 	// waiting := router.WhenID(runtime.id, started)

// 	// <-waiting
// 	fmt.Println("started")

// 	// router.Close()
// 	// router.Wait()
// }

// func TestRouterTimeout(t *testing.T) {
// 	router := newRouter(RouterWithTimeout(3 * time.Second))
// 	router.Wait()
// }

// func TestRouterPingPong(t *testing.T) {

// 	router := newRouter(RouterWithTimeout(5 * time.Second))

// 	counter := 0

// 	runtimePing := newRuntime("ping", RuntimeWithID(1), RuntimeWithRuntime(testPing(&counter)))
// 	router.Add(*runtimePing) // gronos manage IDs, the router doesn't know

// 	runtimePong := newRuntime("pong", RuntimeWithID(2), RuntimeWithRuntime(testPong(&counter)))
// 	router.Add(*runtimePong) // gronos manage IDs, the router doesn't know

// 	<-router.WhenID(runtimePing.id, started)
// 	<-router.WhenID(runtimePong.id, started)

// 	if err := router.named("pong", "ping"); err != nil {
// 		t.Fatalf("Error naming pong: %v\n", err)
// 	}

// 	go func() {
// 		t := time.NewTimer(3 * time.Second)
// 		<-t.C
// 		slog.Info("trigger close")
// 		router.Close()
// 	}()

// 	router.Wait()
// }

// func TestNoRuntime(t *testing.T) {
// 	router := newRouter(RouterWithTimeout(3 * time.Second))

// 	go func() {
// 		t := time.NewTimer(2 * time.Second)
// 		<-t.C
// 		router.Close()
// 	}()

// 	router.Wait()
// }
