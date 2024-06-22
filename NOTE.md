When discovering a new feature or testing something new, starts from the lowest hierarchial module first.

It is a very hierarchial structure, each level responsible for their own set of features.


I have yet to figure it out to manage the API for the clocks 

I love the new api for the context
- withXXX for internal init
- useXXX for internal usage
- UseXXX for public usage within the runtime

I also appreciate the minimal Runtime type now
- just `func(ctx context.Context)` 


What's missing:
- how to manage CLOCKS => watchmaker?
- how to manage courier to router

It is also missing the general API experience, I want it to be as slick as possible to which it make sense! It has to have a good vibe to it.


To me `gronos` is the missing piece of the one-runtime domain driven design that assemble the pieces of that puzzle together into one cohesive runtime so each parts can be loosely coupled. You should compose your application with multiple runtimes.



```go

// TODO: how to name the variable?!
box := gronos.New()

// each runtime, aren't directly connected to each others, they send a message to the router through the Courier, the router will 
// analyze and dispatch the messages to each runtimes OR trigger internal behaviors
runtimeInfo := box.Add(
    func(ctx context.Context) error {

        shutdown, _ := UseShutdown(ctx)

        <-shutdown.Await()

        return nil
    },
    RuntimeWithMailboxFilters(
        // Specify the types of messages you're allowing into the mailbox
        gronos.Accept(
            gronos.MessageType[msgSomething]()
            gronos.MessageType[msgOther]()
            gronos.MessageType[msgWeird]()
        ),
        gronos.Reject(
            gronos.MessageType[msgTicker]()
        ),
    ),
    RuntimeWithMailboxClock(
        gronos.NewClock(time.Second/60), // speed of message reading - clock will be added to the watchmaker
    ),
    RuntimeWithCourier(
        gronos.NewCourier(
            gronos.CourierWithClock(time.Second/30), // speed of message delivery - clock will be added to the watchmaker
        ),
    ),
)

{
    box.RuntimeClockReset(time.Second/120) // TODO: function to change the clock of any parts

    // TODO: gronos should propose runtime extensions to allow data extractions or some kind of internal modules
    // box.Add(middlewares.Prometheus()) ? something like that?  idk man 
}

_ := runtimeInfo // contains the ID, states, triggers, metrics, ...

go func() {
    time.Sleep(time.Second)
    box.Stop()
}()

<- box.Start()

```


--- 


A lot of new ideas to make it easier to maintain comes to my mind

A another way to prepare to setup everything

- Runtime Key
    - Port Key

Mean that you need both the runtime and port key to be able to send messages 

```go

type workerPortKey int 

const (
    portA workerPortKey = iota
    portB
)

// the idea is to `UseXXX` context while we `WithXXX` context internally automatically 
// TODO I might have conflicts because while the runtime is running, there might not be update possible
func testRuntime(ctx context.Context) error {
    port, ok := gronos.UsePort(workerKey, portB)
    if !ok {
        // something
    }
    port.Push(Something{})

	shutdown, _ := UseShutdown(ctx)
	mailbox, _ := UseMailbox(ctx)

	for {
		select {
		case msg := <-mailbox:
			slog.Info("test runtime msg: ", slog.Any("msg", msg))
		case <-ctx.Done():
			slog.Info("test runtime ctx.Done()")
			return nil
		case <-shutdown.Await():
			slog.Info("test runtime shutdown")
			// courier.Transmit(fmt.Errorf("shutdown"))
			return nil
		}
	}

	return fmt.Errorf("test runtime error")
}

type runtimeKey int

const (
    httpKey runtimeKey = iota
    workerKey
)

// instead we ask for keys for each runtime
gns, _ := gronos.New[runtimeKey]()

// compacted runtime with cancellation
id, cancel, := gns.Add(
    httpKey,
    testRuntime, // no need for func
    gronos.WithTimeout(time.Second*3), // runtime.WithTimeout wrapped
    gronos.WithValue("xx", "yy"), // runtime.WithValue wrapped
    gronos.WithPort( /* blablabla */ )
)

ctx := context.Background()
ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
defer cancel()

signal, receiver := gns.Run(ctx)

// wait until the runtimes decide to stop or a big panic
for {
	select {
	case <-signal.Await():
		slog.Info("signal")
        break
	case err := <-receiver:
		slog.Info("error: ", err)
        break
	case <-ctx.Done():
		slog.Info("ctx.Done()")
		cancel()
        break
	}
}

<- g.GracefullWait()

```

Now the next questions i will hve will be about how do I manage the behind the scene thing, probably i will have to add up some context.

