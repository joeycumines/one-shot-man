package jscompliance

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// echoDescriptorSet builds a minimal proto3 FileDescriptorSet:
//
//	package example;
//	message EchoRequest  { string message = 1; }
//	message EchoResponse { string message = 1; }
//	service EchoService  { rpc Echo(EchoRequest) returns (EchoResponse); }
//
// (mirrors goja-grpc's exampleDescBytes), marshaled for osm:protobuf.
func echoDescriptorSet() []byte {
	str := func(s string) *string { return &s }
	t := descriptorpb.FieldDescriptorProto_TYPE_STRING
	l := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{{
			Name:    str("example.proto"),
			Package: str("example"),
			Syntax:  str("proto3"),
			MessageType: []*descriptorpb.DescriptorProto{{
				Name: str("EchoRequest"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: str("message"), Number: proto.Int32(1), Type: t.Enum(), Label: l.Enum(), JsonName: str("message")},
				},
			}, {
				Name: str("EchoResponse"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: str("message"), Number: proto.Int32(1), Type: t.Enum(), Label: l.Enum(), JsonName: str("message")},
				},
			}},
			Service: []*descriptorpb.ServiceDescriptorProto{{
				Name: str("EchoService"),
				Method: []*descriptorpb.MethodDescriptorProto{{
					Name:       str("Echo"),
					InputType:  str(".example.EchoRequest"),
					OutputType: str(".example.EchoResponse"),
				}},
			}},
		}},
	}
	data, err := proto.Marshal(fds)
	if err != nil {
		panic(err)
	}
	return data
}

// TestSlow_Grpc_RoundTrip is the grpc behavioral round-trip (the biggest
// remaining coverage gap — the whole RPC transport was unexercised). It loads a
// minimal Echo service descriptor, registers a JS server handler, creates a
// client, calls Echo, and asserts the response VALUE round-trips through the
// in-process gRPC channel. Exercises osm:grpc + osm:protobuf + the inproc
// channel end-to-end.
func TestSlow_Grpc_RoundTrip(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)

	// Load the descriptor bytes into osm:protobuf (set as a global, then JS
	// passes them to loadDescriptorSet).
	desc := echoDescriptorSet()
	if err := engine.Loop().Submit(func() {
		_ = engine.Runtime().Set("__desc", desc)
	}); err != nil {
		t.Fatalf("set descriptor global: %v", err)
	}

	v, err := evalJS(t, engine, `(async function () {
		var pb = require('osm:protobuf');
		var grpc = require('osm:grpc');
		pb.loadDescriptorSet(globalThis.__desc);
		var server = grpc.createServer();
		server.addService('example.EchoService', {
			echo: function (request, call) {
				var resp = new (pb.messageType('example.EchoResponse'))();
				resp.set('message', request.get('message'));
				return resp;
			}
		});
		server.start();
		var client = grpc.createClient('example.EchoService');
		var req = new (pb.messageType('example.EchoRequest'))();
		req.set('message', 'jscompliance-grpc-roundtrip');
		var resp = await client.echo(req);
		return resp.get('message');
	})()`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("grpc round-trip failed: %v", err)
	}
	if s, _ := v.(string); s != "jscompliance-grpc-roundtrip" {
		t.Errorf("grpc echo round-trip = %q, want 'jscompliance-grpc-roundtrip'", s)
	}
}
