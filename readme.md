# Gronos

> If Chronos is the god of time
> Then Gronos is the god of runtimes

Gronos is a Go package that provides a simple and flexible runtime management system. It allows you to create and manage multiple runtimes, each with its own context, mailbox, and courier. Runtimes can communicate with each other by sending and receiving messages, and they can also notify the central station about errors or shutdown signals.

- Organizing our code can be challenging
- Leveraging DDD mean leaveraging one or more transporters and eventually having workers within the same application
- Multiple side runtimes across different goroutines can be challenging, it is a waste of time
- Focus on allowing an easy organization of the code or domains when composed on the runtime application layer
- Provide structures to communicate and assemble domains/modules into one coherent runtime application

## Features

- Create and manage multiple runtimes
- Inter-runtime communication through message passing
- Error handling and shutdown signaling
- Context management with cancellation, timeout, and deadline
- Graceful and immediate shutdown modes
- Optional: Timed and cron-like task execution (I will add more middlewares like that)
- 

## Installation

```
go get github.com/davidroman0O/gronos
```

## Usage

Here's a basic example of how to use Gronos:

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/davidroman0O/gronos"
)

func main() {
	// Create a new Gronos station
	g, err := gronos.New(
        gronos.WithGracefullShutdown(), // Choose between Immediate or Gracefull
    )
	if err != nil {
		slog.Info("Error creating new context: ", err)
		return
	}

	// Add a runtime with a timeout and a value
	_, clfn := g.Add(
		gronos.WithRuntime(myRuntime),
		gronos.WithTimeout(time.Second*5),
		gronos.WithValue("myKey", "myValue"),
	)
	defer clfn() // Cancel the context when done

	// Run the station and get the shutdown signal and error channel
	signal, errChan := g.Run(context.Background())

	// Wait for the shutdown signal or an error
	select {
	case <-signal.Await():
		slog.Info("Shutdown signal received")
	case err := <-errChan:
		slog.Info("Error occurred: ", err)
	}

	// Wait for all runtimes to finish
	g.Wait()
}

func myRuntime(ctx context.Context, mailbox *gronos.Mailbox, courier *gronos.Courier, shutdown *gronos.Signal) error {
	// Your runtime logic here
	fmt.Println("My runtime is running")

	// Wait for context cancellation or shutdown signal
	select {
	case <-ctx.Done():
		return nil
	case <-shutdown.Await():
		return nil
	}
}
```

# Inter-Runtime Communication

Gronos allows runtimes to communicate with each other by sending and receiving messages. Here's an example of two runtimes, pingRuntime and pongRuntime, that send messages back and forth:

```go
func pingRuntime(ctx context.Context, mailbox *gronos.Mailbox, courier *gronos.Courier, shutdown *gronos.Signal) error {
	pongID := 2 // Assuming pongRuntime has ID 2

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-shutdown.Await():
			return nil
		default:
			courier.Deliver(gronos.Envelope{
				To:  pongID,
				Msg: "ping",
			})
			slog.Info("Sent 'ping' message to pongRuntime")
		}
	}
}

func pongRuntime(ctx context.Context, mailbox *gronos.Mailbox, courier *gronos.Courier, shutdown *gronos.Signal) error {
	pingID := 1 // Assuming pingRuntime has ID 1

	for {
		select {
		case msg := <-mailbox.Read():
			slog.Info("Received message: ", msg.Msg)
			courier.Deliver(gronos.Envelope{
				To:  pingID,
				Msg: "pong",
			})
			slog.Info("Sent 'pong' message to pingRuntime")
		case <-ctx.Done():
			return nil
		case <-shutdown.Await():
			return nil
		}
	}
}
```

# HTTP Server and Worker Runtime

Gronos can be used to run an HTTP server and a worker runtime in separate runtimes, allowing them to communicate and share tasks. Here's an example:

```go
func main() {
	// Create a new Gronos station
	g, err := gronos.New()
	if err != nil {
		slog.Info("Error creating new context: ", err)
		return
	}

	// Add an HTTP server runtime
	_, _ = g.Add(
		gronos.WithRuntime(httpServerRuntime),
	)

	// Add a worker runtime
	_, _ = g.Add(
		gronos.WithRuntime(workerRuntime),
	)

	// Run the station and get the shutdown signal and error channel
	signal, errChan := g.Run(context.Background())

	// Wait for the shutdown signal or an error
	select {
	case <-signal.Await():
		slog.Info("Shutdown signal received")
	case err := <-errChan:
		slog.Info("Error occurred: ", err)
	}

	// Wait for all runtimes to finish
	g.Wait()
}

func httpServerRuntime(ctx context.Context, mailbox *gronos.Mailbox, courier *gronos.Courier, shutdown *gronos.Signal) error {
	workerID := 2 // Assuming workerRuntime has ID 2

	// Start an HTTP server
	http.HandleFunc("/task", func(w http.ResponseWriter, r *http.Request) {
		// Get the task from the request
		task := r.FormValue("task")

		// Send the task to the worker runtime
		courier.Deliver(gronos.Envelope{
			To:  workerID,
			Msg: task,
		})
		slog.Info("Sent task '", task, "' to workerRuntime")

		// Respond with a success message
		fmt.Fprintf(w, "Task '%s' sent to worker", task)
	})

	// Start the HTTP server
	slog.Info("Starting HTTP server on :8080")
	go http.ListenAndServe(":8080", nil)

	// Wait for context cancellation or shutdown signal
	select {
	case <-ctx.Done():
		return nil
	case <-shutdown.Await():
		return nil
	}
}

func workerRuntime(ctx context.Context, mailbox *gronos.Mailbox, courier *gronos.Courier, shutdown *gronos.Signal) error {
	for {
		select {
		case msg := <-mailbox.Read():
			task := msg.Msg.(string)
			slog.Info("Received task: ", task)

			// Process the task
			processTask(task)
		case <-ctx.Done():
			return nil
		case <-shutdown.Await():
			return nil
		}
	}
}

func processTask(task string) {
	// Your task processing logic here
	slog.Info("Processing task: ", task)
}
```

In this example, the httpServerRuntime starts an HTTP server that listens for incoming tasks. When a task is received, it sends the task to the workerRuntime using the courier. The workerRuntime receives the task and processes it using the processTask function.
For more advanced usage and examples, please refer to the package documentation.