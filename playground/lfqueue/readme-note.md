
Semi Lock-Free Queue
- Constructor NewLFQueue accepts configuration options `(opts ...Options)`.
- Constructor NewActivateLFQueue accepts an activation function `(activation func(interface{}) bool)` and configuration options.
- Options are:
  - size of a node, default is 128
- Do not implement Dequeue or DequeueN, only a channel to read when data is available
- the data available channel is batching all data, only array of data is accepted
- Use `sync/atomic` for all operations on intergers
- Implement Enqueue and EnqueueN, both functions do not re-use each other
- Queue start with one node and new node are added toward the front as needed
- data are stored from back to front
- Each Tick, data are retrieved from the back and new data fill nodes at the front
- Emptied nodes are reused at the front for new data.
- ONLY USE A MUTEX WHEN THERE'S ONE NODE to prevent race conditions during enqueueing
- On Enqueue, EnqueueN and Tick, compute the amount of nodes available
- On a Tick, empty a node at the back, if the queue got an activation function then filter the list of data into two arrays of activated or non-activated then enqueueN the non-activated then send to the channel, if the queue doesn't not have an activation function then it just push the data into the channel
