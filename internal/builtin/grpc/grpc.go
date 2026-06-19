// Package grpc provides the Goja module "osm:grpc".
//
// It wraps github.com/joeycumines/goja-grpc to expose promise-based,
// in-process gRPC clients and servers to JavaScript. The underlying transport is
// github.com/joeycumines/go-inprocgrpc and executes on the event loop, so the
// module is fully non-blocking.
//
// The wrapper requires three dependencies:
//   - ch: an in-process gRPC channel
//   - pb: the shared protobuf module
//   - adapter: the event loop adapter used for promise-based async operations
package grpc

import (
	"context"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	gojagrpc "github.com/joeycumines/goja-grpc"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
)

// Require returns a module loader for "osm:grpc" backed by goja-grpc.
//
// ctx is the base context for the module; module loading is cancelled if ctx
// is already done. The dependencies ch, pb and adapter are required.
func Require(ctx context.Context, ch *inprocgrpc.Channel, pb *gojaprotobuf.Module, adapter *gojaeventloop.Adapter) require.ModuleLoader {
	loader := gojagrpc.Require(
		gojagrpc.WithChannel(ch),
		gojagrpc.WithProtobuf(pb),
		gojagrpc.WithAdapter(adapter),
	)
	return func(runtime *goja.Runtime, module *goja.Object) {
		if err := ctx.Err(); err != nil {
			panic(runtime.NewGoError(err))
		}
		loader(runtime, module)
	}
}
