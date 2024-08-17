package envelope

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/davidroman0O/gronos"
)

type mockMsg struct {
	Message string `json:"message"`
}

func TestSimpleRegistry(t *testing.T) {

	module := New[string]()
	module.register(Register[mockMsg]())

	evt := mockMsg{
		Message: "Hello, World!",
	}

	// imagine the message is coming from a message broker
	msgBytes, err := json.Marshal(evt)
	if err != nil {
		t.Errorf("expected to marshal the message")
	}

	// has to be mapped to the type signature
	var ok bool
	var signatureAny interface{}
	if signatureAny, ok = module.registry.Load(signatureFor[mockMsg]()); !ok {
		t.Errorf("expected to find the type")
	}

	// we have to cast the interface to a reflect.Type
	var signature reflect.Type = signatureAny.(reflect.Type)

	// we have to create a new instance of the type
	empty := reflect.New(signature).Interface()
	// we have to unmarshal the message into the new instance
	if err := json.Unmarshal(msgBytes, &empty); err != nil {
		t.Errorf("expected to unmarshal the message")
	}
	// we have to cast the interface to the actual type
	empty = reflect.ValueOf(empty).Elem().Interface()

	if empty.(mockMsg).Message != evt.Message {
		t.Errorf("expected to have the same message")
	}

	var abstractPayload interface{} = reflect.ValueOf(empty).Interface()

	fmt.Println(abstractPayload, reflect.TypeOf(abstractPayload), evt)

	switch payload := abstractPayload.(type) {
	case mockMsg:
		if payload.Message != evt.Message {
			t.Errorf("expected to have the same message")
		}
	default:
		t.Errorf("expected to have the same type")
	}
}

func TestMsgRegistry(t *testing.T) {

	module := New[string]()

	confirm, msg := MsgAdd[string](Register[mockMsg]())
	module.OnMsg(context.Background(), &gronos.MessagePayload{Message: msg})
	<-confirm

	evt := mockMsg{
		Message: "Hello, World!",
	}

	// imagine the message is coming from a message broker
	msgBytes, err := json.Marshal(evt)
	if err != nil {
		t.Errorf("expected to marshal the message")
	}

	// has to be mapped to the type signature
	var ok bool
	var signatureAny interface{}
	if signatureAny, ok = module.registry.Load(signatureFor[mockMsg]()); !ok {
		t.Errorf("expected to find the type")
	}

	// we have to cast the interface to a reflect.Type
	var signature reflect.Type = signatureAny.(reflect.Type)

	// we have to create a new instance of the type
	empty := reflect.New(signature).Interface()
	// we have to unmarshal the message into the new instance
	if err := json.Unmarshal(msgBytes, &empty); err != nil {
		t.Errorf("expected to unmarshal the message")
	}
	// we have to cast the interface to the actual type
	empty = reflect.ValueOf(empty).Elem().Interface()

	if empty.(mockMsg).Message != evt.Message {
		t.Errorf("expected to have the same message")
	}

	var abstractPayload interface{} = reflect.ValueOf(empty).Interface()

	fmt.Println(abstractPayload, reflect.TypeOf(abstractPayload), evt)

	switch payload := abstractPayload.(type) {
	case mockMsg:
		if payload.Message != evt.Message {
			t.Errorf("expected to have the same message")
		}
	default:
		t.Errorf("expected to have the same type")
	}
}
