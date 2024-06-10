test-ringbuffer-add:
	go test -v -timeout 5s -run ^TestAdd$$
test-ringbuffer-shared:
	go test -v -timeout 5s -run ^TestShared$$

test-ringbuffer-active-buffer:
	go test -v -timeout 5s -run ^TestNewActiveRingBuffer$$

test-ringbuffer-active-push:
	go test -v -timeout 5s -run ^TestActiveRingBufferPush$$
test-ringbuffer-active-tick:
	go test -v -timeout 5s -run ^TestActiveRingBufferTick$$
test-ringbuffer-active-resize:
	go test -v -timeout 5s -run ^TestActiveRingBufferResize$$
test-ringbuffer-active-close:
	go test -v -timeout 5s -run ^TestActiveRingBufferClose$$

test-ringbuffer-active: test-ringbuffer-active-buffer test-ringbuffer-active-push test-ringbuffer-active-tick test-ringbuffer-active-resize test-ringbuffer-active-close

test-ringbuffer: test-ringbuffer-add test-ringbuffer-shared test-ringbuffer-active-buffer test-ringbuffer-active-push test-ringbuffer-active-tick test-ringbuffer-active-resize test-ringbuffer-active-close

test-ringbuffer:
	go test -v -timeout 5s -run ^TestRuntimeLifecycleBasicTimeout$$

test-runtime-timeout:
	go test -v -timeout 5s -run ^TestRuntimeLifecycleBasicTimeout$$

test-runtime-shutdown:
	go test -v -timeout 5s -run ^TestRuntimeLifecycleBasicShutdown$$

test-runtime-panic:
	go test -v -timeout 5s -run ^TestRuntimeLifecyclePanic$$

test-runtime-courier-basic:
	go test -v -timeout 5s -run ^TestRuntimeCourierBasic$$

test-runtime-mailbox-basic:
	go test -v -timeout 5s -run ^TestRuntimeMailboxBasic$$

test-registry-basic:
	go test -v -timeout 5s -run ^TestRuntimeBasic$$

test-registry-panic:
	go test -v -timeout 5s -run ^TestRegistryPanic$$

test-runtime: test-runtime-timeout test-runtime-shutdown test-runtime-panic test-runtime-courier-basic test-runtime-mailbox-basic

test-simple:
	go test -v -timeout 5s -run ^TestSimple$$

test-router-started:
	go test -v -timeout 5s -run ^TestRouterWhenStarted$$

test-router-pingpong:
	go test -v -timeout 5s -run ^TestRouterPingPong$$

test-router-noruntime:
	go test -v -timeout 5s -run ^TestNoRuntime$$
	
test-router-timeout:
	go test -v -timeout 5s -run ^TestRouterTimeout$$

test-router: test-router-started test-router-pingpong

test-queue-basic:
	go test -v -timeout 5s -run ^TestQueue$$
test-queue-putGet:
	go test -v -timeout 5s -run ^TestQueuePutGet$$
test-queue-general:
	go test -v -timeout 5s -run ^TestQueueGeneral$$
test-queue-putGoGet:
	go test -v -timeout 5s -run ^TestQueuePutGoGet$$
test-queue-putDoGet:
	go test -v -timeout 5s -run ^TestQueuePutDoGet$$
test-queue-putGetOrder:
	go test -v -timeout 5s -run ^TestQueuePutGetOrder$$

test-queue: test-queue-basic test-queue-putGet test-queue-general test-queue-putGoGet test-queue-putDoGet test-queue-putGetOrder

test:
	go test -v -count=1 ./...


test-watchmaker-simple:
	go test -v -timeout 5s -run ^TestWatchMakerSimple$$

test-watchmaker-lifecycle:
	go test -v -timeout 5s -run ^TestWatchMakerLifecycle$$