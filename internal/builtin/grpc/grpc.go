// Package grpc provides a Goja module wrapping Google's gRPC client for JS scripts.
// It is registered as "osm:grpc" and provides synchronous gRPC client operations
// for connecting to external gRPC servers (such as MacosUseSDK).
//
// This module uses google.golang.org/grpc directly rather than goja-grpc, because
// the current osm scripting engine uses dop251/goja_nodejs/eventloop which is
// incompatible with the go-eventloop required by goja-grpc. Once the event loop
// subsystem is migrated to go-eventloop, this module can be replaced with a
// thin wrapper around goja-grpc for full promise-based async support.
//
// JavaScript API:
//
//	const grpc = require('osm:grpc');
//
//	// Load proto descriptors (base64-encoded FileDescriptorSet)
//	grpc.loadDescriptorSet(base64String);
//
//	// Connect to gRPC server
//	const conn = grpc.dial('localhost:50051', { insecure: true });
//
//	// Make unary RPC call — request/response are plain JS objects
//	const resp = conn.invoke('/package.Service/Method', { field: 'value' });
//
//	// Close connection
//	conn.close();
//
//	// Status code constants
//	grpc.status.OK            // 0
//	grpc.status.CANCELLED     // 1
//	grpc.status.NOT_FOUND     // 5
package grpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/dop251/goja"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Module wraps gRPC client functionality for JavaScript scripts.
// It maintains a registry of loaded proto descriptors for message encoding.
type Module struct {
	mu    sync.RWMutex
	files *descriptorFiles
}

// descriptorFiles tracks loaded proto file descriptors.
type descriptorFiles struct {
	// services maps fully-qualified service names to their method descriptors.
	// Built from loaded FileDescriptorSets.
	services map[string]serviceInfo
}

// serviceInfo holds resolved method descriptors for a service.
type serviceInfo struct {
	methods map[string]protoreflect.MethodDescriptor
}

// Require returns a module loader for osm:grpc that uses the provided base context
// for all gRPC operations. The context is used as the parent for RPC calls,
// ensuring cancellation propagation when the script terminates.
func Require(ctx context.Context) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		m := &Module{
			files: &descriptorFiles{
				services: make(map[string]serviceInfo),
			},
		}
		exports := module.Get("exports").(*goja.Object)
		_ = exports.Set("dial", runtime.ToValue(m.jsDial(runtime, ctx)))
		_ = exports.Set("loadDescriptorSet", runtime.ToValue(m.jsLoadDescriptorSet(runtime)))
		_ = exports.Set("status", m.jsStatus(runtime))
	}
}

// jsLoadDescriptorSet loads a base64-encoded FileDescriptorSet into the module's
// proto registry. Services and their methods become available for invoke() calls.
//
// JS: grpc.loadDescriptorSet(base64String)
func (m *Module) jsLoadDescriptorSet(runtime *goja.Runtime) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0)
		if goja.IsUndefined(arg) || goja.IsNull(arg) {
			panic(runtime.NewTypeError("loadDescriptorSet: argument must be a base64-encoded FileDescriptorSet"))
		}

		b64 := arg.String()
		data, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			panic(runtime.NewGoError(fmt.Errorf("loadDescriptorSet: invalid base64: %w", err)))
		}

		var fds descriptorpb.FileDescriptorSet
		if err := proto.Unmarshal(data, &fds); err != nil {
			panic(runtime.NewGoError(fmt.Errorf("loadDescriptorSet: invalid FileDescriptorSet: %w", err)))
		}

		files, err := protodesc.NewFiles(&fds)
		if err != nil {
			panic(runtime.NewGoError(fmt.Errorf("loadDescriptorSet: failed to parse descriptors: %w", err)))
		}

		m.mu.Lock()
		defer m.mu.Unlock()

		// Walk all files and register services
		files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
			svcs := fd.Services()
			for i := 0; i < svcs.Len(); i++ {
				svc := svcs.Get(i)
				svcName := string(svc.FullName())
				si := serviceInfo{methods: make(map[string]protoreflect.MethodDescriptor)}
				methods := svc.Methods()
				for j := 0; j < methods.Len(); j++ {
					md := methods.Get(j)
					si.methods[string(md.Name())] = md
				}
				m.files.services[svcName] = si
			}
			return true
		})

		return goja.Undefined()
	}
}

// jsDial creates a gRPC client connection.
//
// JS: grpc.dial(target, opts?)
//
//	target: string — server address (e.g., "localhost:50051")
//	opts: {
//	  insecure?: boolean  — use plaintext (no TLS)
//	  authority?: string  — override :authority header
//	}
//
// Returns a connection object with:
//
//	invoke(method, request?)  — make unary RPC call
//	close()                   — close the connection
//	target                    — the connection target string
func (m *Module) jsDial(runtime *goja.Runtime, ctx context.Context) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		target := call.Argument(0).String()
		if target == "" {
			panic(runtime.NewTypeError("dial: target must be a non-empty string"))
		}

		var dialOpts []grpc.DialOption

		optsArg := call.Argument(1)
		if !goja.IsUndefined(optsArg) && !goja.IsNull(optsArg) {
			if opts, ok := optsArg.Export().(map[string]interface{}); ok {
				if v, ok := opts["insecure"]; ok {
					if b, ok := v.(bool); ok && b {
						dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
					}
				}
				if v, ok := opts["authority"]; ok {
					if s, ok := v.(string); ok {
						dialOpts = append(dialOpts, grpc.WithAuthority(s))
					}
				}
			}
		}

		conn, err := grpc.NewClient(target, dialOpts...)
		if err != nil {
			panic(runtime.NewGoError(fmt.Errorf("dial: %w", err)))
		}

		return m.jsConnObject(runtime, ctx, conn, target)
	}
}

// jsConnObject creates a JS object wrapping a gRPC client connection with
// invoke() and close() methods.
func (m *Module) jsConnObject(runtime *goja.Runtime, ctx context.Context, conn *grpc.ClientConn, target string) goja.Value {
	var closeOnce sync.Once

	obj := runtime.NewObject()
	_ = obj.Set("invoke", runtime.ToValue(m.jsInvoke(runtime, ctx, conn)))
	_ = obj.Set("close", runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		var closeErr error
		closeOnce.Do(func() {
			closeErr = conn.Close()
		})
		if closeErr != nil {
			panic(runtime.NewGoError(fmt.Errorf("close: %w", closeErr)))
		}
		return goja.Undefined()
	}))
	_ = obj.Set("target", runtime.ToValue(target))

	return obj
}

// jsInvoke makes a synchronous unary RPC call.
//
// JS: conn.invoke(method, request?)
//
//	method: string — full gRPC method path (e.g., "/package.Service/Method")
//	request: object? — request fields as a plain JS object
//
// Returns: object — response fields as a plain JS object
//
// Throws GrpcError with code and message on RPC failure.
func (m *Module) jsInvoke(runtime *goja.Runtime, ctx context.Context, conn *grpc.ClientConn) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		method := call.Argument(0).String()
		if method == "" {
			panic(runtime.NewTypeError("invoke: method must be a non-empty string (e.g., '/package.Service/Method')"))
		}

		// Parse method path: /package.Service/MethodName
		svcName, methodName, err := parseFullMethod(method)
		if err != nil {
			panic(runtime.NewGoError(fmt.Errorf("invoke: %w", err)))
		}

		// Look up method descriptor
		m.mu.RLock()
		si, ok := m.files.services[svcName]
		if !ok {
			m.mu.RUnlock()
			panic(runtime.NewGoError(fmt.Errorf("invoke: service %q not found in loaded descriptors; call loadDescriptorSet() first", svcName)))
		}
		md, ok := si.methods[methodName]
		if !ok {
			m.mu.RUnlock()
			panic(runtime.NewGoError(fmt.Errorf("invoke: method %q not found in service %q", methodName, svcName)))
		}
		m.mu.RUnlock()

		// Build request message
		reqMsg := dynamicpb.NewMessage(md.Input())
		reqArg := call.Argument(1)
		if !goja.IsUndefined(reqArg) && !goja.IsNull(reqArg) {
			jsonBytes, err := json.Marshal(reqArg.Export())
			if err != nil {
				panic(runtime.NewGoError(fmt.Errorf("invoke: failed to marshal request to JSON: %w", err)))
			}
			if err := protojson.Unmarshal(jsonBytes, reqMsg); err != nil {
				panic(runtime.NewGoError(fmt.Errorf("invoke: failed to convert request to proto: %w", err)))
			}
		}

		// Build response message
		respMsg := dynamicpb.NewMessage(md.Output())

		// Make the RPC call (blocking)
		rpcCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		if err := conn.Invoke(rpcCtx, method, reqMsg, respMsg); err != nil {
			// Create a rich error with gRPC status information
			if st, ok := grpcstatus.FromError(err); ok {
				errObj := runtime.NewObject()
				_ = errObj.Set("name", "GrpcError")
				_ = errObj.Set("code", int(st.Code()))
				_ = errObj.Set("message", st.Message())
				panic(errObj)
			}
			panic(runtime.NewGoError(fmt.Errorf("invoke: %w", err)))
		}

		// Convert response back to JS object
		respJSON, err := protojson.Marshal(respMsg)
		if err != nil {
			panic(runtime.NewGoError(fmt.Errorf("invoke: failed to marshal response: %w", err)))
		}

		var result interface{}
		if err := json.Unmarshal(respJSON, &result); err != nil {
			panic(runtime.NewGoError(fmt.Errorf("invoke: failed to parse response JSON: %w", err)))
		}

		return runtime.ToValue(result)
	}
}

// jsStatus creates a JS object containing all gRPC status code constants.
func (m *Module) jsStatus(runtime *goja.Runtime) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("OK", int(codes.OK))
	_ = obj.Set("CANCELLED", int(codes.Canceled))
	_ = obj.Set("UNKNOWN", int(codes.Unknown))
	_ = obj.Set("INVALID_ARGUMENT", int(codes.InvalidArgument))
	_ = obj.Set("DEADLINE_EXCEEDED", int(codes.DeadlineExceeded))
	_ = obj.Set("NOT_FOUND", int(codes.NotFound))
	_ = obj.Set("ALREADY_EXISTS", int(codes.AlreadyExists))
	_ = obj.Set("PERMISSION_DENIED", int(codes.PermissionDenied))
	_ = obj.Set("RESOURCE_EXHAUSTED", int(codes.ResourceExhausted))
	_ = obj.Set("FAILED_PRECONDITION", int(codes.FailedPrecondition))
	_ = obj.Set("ABORTED", int(codes.Aborted))
	_ = obj.Set("OUT_OF_RANGE", int(codes.OutOfRange))
	_ = obj.Set("UNIMPLEMENTED", int(codes.Unimplemented))
	_ = obj.Set("INTERNAL", int(codes.Internal))
	_ = obj.Set("UNAVAILABLE", int(codes.Unavailable))
	_ = obj.Set("DATA_LOSS", int(codes.DataLoss))
	_ = obj.Set("UNAUTHENTICATED", int(codes.Unauthenticated))
	return obj
}

// parseFullMethod parses a gRPC method path of the form "/service.Name/MethodName"
// into its service and method components.
func parseFullMethod(method string) (service, methodName string, err error) {
	if !strings.HasPrefix(method, "/") {
		return "", "", fmt.Errorf("method must start with '/' (got %q)", method)
	}
	parts := strings.SplitN(method[1:], "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("method must be in the form '/service.Name/MethodName' (got %q)", method)
	}
	return parts[0], parts[1], nil
}
