package gronos

import (
	"testing"
)

func TestRouterBasic(t *testing.T) {
	// router := newRouter()

	// router.Add("simple",
	// 	RuntimeWithRuntime(
	// 		func(ctx context.Context, mailbox *Mailbox, courrier *Courier, shutdown *Signal) error {
	// 			for {
	// 				select {
	// 				case <-timer.C:
	// 					panic("lifecycle panic: runtime panic")
	// 				case msg := <-mailbox.Read():
	// 					slog.Info("lifecycle panic: runtime msg: ", slog.Any("msg", msg))
	// 					close(read)
	// 				case <-ctx.Done():
	// 					slog.Info("lifecycle panic: runtime ctx.Done()")
	// 					close(done)
	// 					return nil
	// 				case <-shutdown.Await():
	// 					slog.Info("lifecycle panic: runtime shutdown")
	// 					close(shut)
	// 					return nil
	// 				}
	// 			}
	// 		}))

	// // should start the runtime
	// router.registry.Tick()

	// //
	// router.Tick()

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
