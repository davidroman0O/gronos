package gronos

// Let's reduce the amount of code that manage messages and have a router that allocate one ringbuffer while analyzing the messages
// - plug runtime functions
// - exchange msg between runtime functions
// - analyze metadata in case it's our own messages
// - collect metrics
// The router shouldn't take too much resources or at best you should be able to specify the resources it can take
// A router is targeting Mailboxes and not runtime functions directly
type Router struct {
	ringBuffer *RingBuffer[envelope]
}

type RouterConfig struct {
	initialSize int
	expandable  bool
	throughput  int
}

type RouterOption func(*RouterConfig)

func RouterWithInitialSize(size int) RouterOption {
	return func(c *RouterConfig) {
		c.initialSize = size
	}
}

func RouterWithExpandable(expandable bool) RouterOption {
	return func(c *RouterConfig) {
		c.expandable = expandable
	}
}

func RouterWithThroughput(throughput int) RouterOption {
	return func(c *RouterConfig) {
		c.throughput = throughput
	}
}

func newRouter(opts ...RouterOption) *Router {
	config := &RouterConfig{
		initialSize: 1024,
		expandable:  true,
		throughput:  64,
	}
	for _, v := range opts {
		v(config)
	}
	r := &Router{
		ringBuffer: NewRingBuffer[envelope](config.initialSize, config.expandable, config.throughput),
	}
	return r
}
