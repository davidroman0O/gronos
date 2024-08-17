package envelope

import "encoding/json"

/// You can use regular messages when you have to deal with simple patterns like request/reply, publish/subscribe, etc.
/// But when you need to deal with multiple services with complex return addresses, you need to use Envelopes that will enhance more data in it.
/// Envelops are messages with more metadata to help routers to route the message to the right place so it can eventually return where it was requested to return to.

type metaKey string

func (e metaKey) String() string {
	return string(e)
}

const (
	/// Related to the instance
	metaCreationDate metaKey = "creation_date" // automated
	metaSignature    metaKey = "signature"     // required key of the registry sync.Map
	metaTopic        metaKey = "topic"         // required

	metaProtocol metaKey = "protocol" // (optional) e.g. http, grpc, ws, etc.
	metaUri      metaKey = "uri"      // (optional) e.g. http://localhost:8080
	metaReplyTo  metaKey = "reply_to" // (optional)

	/// Related to the tracing

	// Reference https://www.enterpriseintegrationpatterns.com/patterns/messaging/CorrelationIdentifier.html
	metaCorrelationID metaKey = "correlation_id" // (optional)

	/// Related to the stream

	// Reference https://www.enterpriseintegrationpatterns.com/patterns/messaging/ReturnAddress.html
	metaReturnAddress metaKey = "return_address" // (optional) which is the the name of the topic generally
	metaReturnUri     metaKey = "return_uri"     // (optional) e.g. http://localhost:8080
)

// https://www.enterpriseintegrationpatterns.com/patterns/messaging/EnvelopeWrapper.html
// An envelope will simply wrap a message with metadata of the Message's payload allowing other topics to identify it as an envelope and therefore being able to deserialize it with more type-safety.
// You will have contorl on switching on which type you pre-defined for a topic BUT you will discard the unknown types.
type Envelope struct {
	metadata map[string]string // watermill inspired
	payload  interface{}       // runtime payload rendered by it's `message`
	message  []byte            // watermill inspired, used for (future) network transport
}

// Return fundamental data types for any third parties (e.g. Kafka, RabbitMQ, etc.)
func (e *Envelope) AsMessage() (map[string]string, []byte, error) {
	asbytes, err := json.Marshal(e.payload)
	return e.metadata, asbytes, err
}

func (e *Envelope) Payload() interface{} {
	return e.payload
}
