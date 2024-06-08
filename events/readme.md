There is a need to pre-parse messages into events by knowing in advance all possible structs an entry of events.

Since it is intended to have multiple "Runtimes" they will need to exchange some in and out messages. Those messages need to be known in advance to perform diverse computation, control, parsing, metrics, etc.

Each Runtime can define their own gateway with set of structs while listening to each entries. 

The registry will own all gateways to each the publishing of events for each "Runtime".

I think that each gateway need to accumulate messages like my ringbuffer and a clock will move the messages.


Event loop that pull messages from ports to router
