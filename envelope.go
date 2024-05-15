package gronos

type Message interface{}

type Metadata map[string]interface{}

type envelope struct {
	to   uint
	name string
	Msg  Message
	Meta Metadata
}

func Envelope(name string, msg Message) envelope {
	return envelope{
		to:   0,
		name: name,
		Msg:  msg,
		Meta: make(Metadata),
	}
}

func EnvelopeMetadata(name string, msg Message, metadata Metadata) envelope {
	return envelope{
		to:   0,
		name: name,
		Msg:  msg,
		Meta: metadata,
	}
}
