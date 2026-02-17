// Package grpc provides a Goja module wrapping goja-grpc for JavaScript scripts.
// It is registered as "osm:grpc" and provides promise-based gRPC client and server
// operations for connecting to external gRPC servers.
//
// This module is a thin wrapper around github.com/joeycumines/goja-grpc.
// It requires three dependencies from the scripting engine:
//   - An in-process gRPC channel (inprocgrpc.Channel)
//   - A shared protobuf module (gojaprotobuf.Module)
//   - An event loop adapter (gojaeventloop.Adapter)
//
// JavaScript API:
//
//	const grpc = require('osm:grpc');
//	const pb = require('osm:protobuf');
//
//	// Load proto descriptors (binary FileDescriptorSet)
//	pb.loadDescriptorSet(descriptorBytes);
//
//	// Create client for a service
//	const client = grpc.createClient('package.Service');
//
//	// Make unary RPC call (returns Promise)
//	const resp = await client.method({ field: 'value' });
//
//	// Dial an external gRPC server
//	const channel = grpc.dial('localhost:50051', { insecure: true });
//
//	// Status code constants
//	grpc.status.OK            // 0
//	grpc.status.CANCELLED     // 1
//	grpc.status.NOT_FOUND     // 5
package grpc

import (
	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	gojagrpc "github.com/joeycumines/goja-grpc"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"

	"github.com/dop251/goja_nodejs/require"
)

// Require returns a module loader for osm:grpc backed by goja-grpc.
// All three dependencies are required:
//   - ch: in-process gRPC channel for RPC communication
//   - pb: shared protobuf module for message encoding/decoding
//   - adapter: event loop adapter for promise-based async operations
func Require(ch *inprocgrpc.Channel, pb *gojaprotobuf.Module, adapter *gojaeventloop.Adapter) require.ModuleLoader {
	return gojagrpc.Require(
		gojagrpc.WithChannel(ch),
		gojagrpc.WithProtobuf(pb),
		gojagrpc.WithAdapter(adapter),
	)
}
