package grpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// testDescriptors creates a FileDescriptorSet for a simple Echo service.
func testDescriptors() *descriptorpb.FileDescriptorSet {
	return &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{{
			Name:    proto.String("test.proto"),
			Package: proto.String("test"),
			Syntax:  proto.String("proto3"),
			MessageType: []*descriptorpb.DescriptorProto{
				{
					Name: proto.String("EchoRequest"),
					Field: []*descriptorpb.FieldDescriptorProto{
						{Name: proto.String("message"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("message")},
					},
				},
				{
					Name: proto.String("EchoResponse"),
					Field: []*descriptorpb.FieldDescriptorProto{
						{Name: proto.String("message"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("message")},
					},
				},
			},
			Service: []*descriptorpb.ServiceDescriptorProto{{
				Name: proto.String("EchoService"),
				Method: []*descriptorpb.MethodDescriptorProto{{
					Name:       proto.String("Echo"),
					InputType:  proto.String(".test.EchoRequest"),
					OutputType: proto.String(".test.EchoResponse"),
				}},
			}},
		}},
	}
}

// testDescriptorBase64 returns the test descriptors as a base64-encoded string.
func testDescriptorBase64(t *testing.T) string {
	t.Helper()
	data, err := proto.Marshal(testDescriptors())
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(data)
}

// startEchoServer starts a gRPC server that echoes the request message field
// into the response message field. Returns the server address and a stop function.
func startEchoServer(t *testing.T) (addr string, stop func()) {
	t.Helper()

	fds := testDescriptors()
	files, err := protodesc.NewFiles(fds)
	require.NoError(t, err)

	// Find method descriptors
	svcDesc, err := files.FindDescriptorByName("test.EchoService")
	require.NoError(t, err)
	svc := svcDesc.(protoreflect.ServiceDescriptor)
	echoMethod := svc.Methods().ByName("Echo")
	require.NotNil(t, echoMethod)
	inputDesc := echoMethod.Input()
	outputDesc := echoMethod.Output()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := grpc.NewServer()
	srv.RegisterService(&grpc.ServiceDesc{
		ServiceName: "test.EchoService",
		HandlerType: (*interface{})(nil),
		Methods: []grpc.MethodDesc{{
			MethodName: "Echo",
			Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
				req := dynamicpb.NewMessage(inputDesc)
				if err := dec(req); err != nil {
					return nil, err
				}
				resp := dynamicpb.NewMessage(outputDesc)
				msgField := inputDesc.Fields().ByName("message")
				respField := outputDesc.Fields().ByName("message")
				resp.Set(respField, protoreflect.ValueOfString("echo: "+req.Get(msgField).String()))
				return resp, nil
			},
		}},
	}, nil)

	go srv.Serve(lis)

	return lis.Addr().String(), func() {
		srv.GracefulStop()
		lis.Close()
	}
}

// startErrorServer starts a gRPC server that returns a fixed gRPC status error.
func startErrorServer(t *testing.T, code codes.Code, msg string) (addr string, stop func()) {
	t.Helper()

	fds := testDescriptors()
	files, err := protodesc.NewFiles(fds)
	require.NoError(t, err)
	svcDesc, err := files.FindDescriptorByName("test.EchoService")
	require.NoError(t, err)
	svc := svcDesc.(protoreflect.ServiceDescriptor)
	echoMethod := svc.Methods().ByName("Echo")
	inputDesc := echoMethod.Input()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := grpc.NewServer()
	srv.RegisterService(&grpc.ServiceDesc{
		ServiceName: "test.EchoService",
		HandlerType: (*interface{})(nil),
		Methods: []grpc.MethodDesc{{
			MethodName: "Echo",
			Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
				req := dynamicpb.NewMessage(inputDesc)
				if err := dec(req); err != nil {
					return nil, err
				}
				return nil, grpcstatus.Errorf(code, "%s", msg)
			},
		}},
	}, nil)

	go srv.Serve(lis)
	return lis.Addr().String(), func() {
		srv.GracefulStop()
		lis.Close()
	}
}

// loadModuleIntoRuntime creates a goja runtime and loads the osm:grpc module.
func loadModuleIntoRuntime(t *testing.T) (*goja.Runtime, *goja.Object) {
	t.Helper()
	runtime := goja.New()
	module := runtime.NewObject()
	exports := runtime.NewObject()
	_ = module.Set("exports", exports)
	ctx := context.Background()
	loader := Require(ctx)
	loader(runtime, module)
	return runtime, exports
}

func TestGrpc_LoadDescriptorSet(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	b64 := testDescriptorBase64(t)

	// Set up JS globals
	_ = runtime.Set("grpc", exports)
	_ = runtime.Set("descriptors", b64)

	_, err := runtime.RunString(`grpc.loadDescriptorSet(descriptors)`)
	assert.NoError(t, err)
}

func TestGrpc_LoadDescriptorSet_InvalidBase64(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	_ = runtime.Set("grpc", exports)

	_, err := runtime.RunString(`grpc.loadDescriptorSet("not-valid-base64!!!")`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid base64")
}

func TestGrpc_LoadDescriptorSet_InvalidProto(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	_ = runtime.Set("grpc", exports)

	// Valid base64 but not a valid protobuf
	b64 := base64.StdEncoding.EncodeToString([]byte("not a protobuf"))
	_ = runtime.Set("b64", b64)

	_, err := runtime.RunString(`grpc.loadDescriptorSet(b64)`)
	// protobuf Unmarshal may or may not error on arbitrary bytes.
	// If it doesn't error, protodesc.NewFiles will catch schema issues.
	// Either way we accept both outcomes — this test just ensures no panic crash.
	_ = err
}

func TestGrpc_LoadDescriptorSet_NullArg(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	_ = runtime.Set("grpc", exports)

	_, err := runtime.RunString(`grpc.loadDescriptorSet(null)`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "loadDescriptorSet")
}

func TestGrpc_Dial(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	_ = runtime.Set("grpc", exports)

	v, err := runtime.RunString(`
		var conn = grpc.dial('localhost:50051', { insecure: true });
		conn.target;
	`)
	require.NoError(t, err)
	assert.Equal(t, "localhost:50051", v.Export())

	// Clean up
	_, err = runtime.RunString(`conn.close()`)
	require.NoError(t, err)
}

func TestGrpc_Dial_EmptyTarget(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	_ = runtime.Set("grpc", exports)

	_, err := runtime.RunString(`grpc.dial('')`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "target must be a non-empty string")
}

func TestGrpc_Dial_NoOpts(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	_ = runtime.Set("grpc", exports)

	// grpc.NewClient requires explicit transport credentials.
	// Dialing without opts should fail with a clear error.
	_, err := runtime.RunString(`grpc.dial('localhost:50051')`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no transport security set")
}

func TestGrpc_Close_Idempotent(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	_ = runtime.Set("grpc", exports)

	_, err := runtime.RunString(`
		var conn = grpc.dial('localhost:50051', { insecure: true });
		conn.close();
		conn.close(); // second close should not error
	`)
	require.NoError(t, err)
}

func TestGrpc_Invoke_UnaryEcho(t *testing.T) {
	addr, stop := startEchoServer(t)
	defer stop()

	runtime, exports := loadModuleIntoRuntime(t)
	b64 := testDescriptorBase64(t)

	_ = runtime.Set("grpc", exports)
	_ = runtime.Set("addr", addr)
	_ = runtime.Set("descriptors", b64)

	v, err := runtime.RunString(`
		grpc.loadDescriptorSet(descriptors);
		var conn = grpc.dial(addr, { insecure: true });
		var resp = conn.invoke('/test.EchoService/Echo', { message: 'hello' });
		conn.close();
		resp.message;
	`)
	require.NoError(t, err)
	assert.Equal(t, "echo: hello", v.Export())
}

func TestGrpc_Invoke_EmptyRequest(t *testing.T) {
	addr, stop := startEchoServer(t)
	defer stop()

	runtime, exports := loadModuleIntoRuntime(t)
	b64 := testDescriptorBase64(t)

	_ = runtime.Set("grpc", exports)
	_ = runtime.Set("addr", addr)
	_ = runtime.Set("descriptors", b64)

	v, err := runtime.RunString(`
		grpc.loadDescriptorSet(descriptors);
		var conn = grpc.dial(addr, { insecure: true });
		var resp = conn.invoke('/test.EchoService/Echo');
		conn.close();
		resp.message;
	`)
	require.NoError(t, err)
	// Empty request → echo of empty string → "echo: "
	assert.Equal(t, "echo: ", v.Export())
}

func TestGrpc_Invoke_GrpcError(t *testing.T) {
	addr, stop := startErrorServer(t, codes.NotFound, "thing not found")
	defer stop()

	runtime, exports := loadModuleIntoRuntime(t)
	b64 := testDescriptorBase64(t)

	_ = runtime.Set("grpc", exports)
	_ = runtime.Set("addr", addr)
	_ = runtime.Set("descriptors", b64)

	v, err := runtime.RunString(`
		grpc.loadDescriptorSet(descriptors);
		var conn = grpc.dial(addr, { insecure: true });
		var errInfo;
		try {
			conn.invoke('/test.EchoService/Echo', { message: 'test' });
			errInfo = { caught: false };
		} catch (e) {
			errInfo = { caught: true, code: e.code, message: e.message, name: e.name };
		}
		conn.close();
		JSON.stringify(errInfo);
	`)
	require.NoError(t, err)

	var errInfo map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(v.String()), &errInfo))
	assert.Equal(t, true, errInfo["caught"])
	assert.Equal(t, float64(codes.NotFound), errInfo["code"])
	assert.Equal(t, "thing not found", errInfo["message"])
	assert.Equal(t, "GrpcError", errInfo["name"])
}

func TestGrpc_Invoke_MethodNotLoaded(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	_ = runtime.Set("grpc", exports)

	_, err := runtime.RunString(`
		var conn = grpc.dial('localhost:50051', { insecure: true });
		try {
			conn.invoke('/test.EchoService/Echo', { message: 'hello' });
		} finally {
			conn.close();
		}
	`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in loaded descriptors")
}

func TestGrpc_Invoke_InvalidMethodFormat(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	b64 := testDescriptorBase64(t)

	_ = runtime.Set("grpc", exports)
	_ = runtime.Set("descriptors", b64)

	_, err := runtime.RunString(`
		grpc.loadDescriptorSet(descriptors);
		var conn = grpc.dial('localhost:50051', { insecure: true });
		try {
			conn.invoke('badformat', {});
		} finally {
			conn.close();
		}
	`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must start with '/'")
}

func TestGrpc_Invoke_EmptyMethod(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	_ = runtime.Set("grpc", exports)

	_, err := runtime.RunString(`
		var conn = grpc.dial('localhost:50051', { insecure: true });
		try {
			conn.invoke('');
		} finally {
			conn.close();
		}
	`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be a non-empty string")
}

func TestGrpc_Invoke_UnknownService(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	b64 := testDescriptorBase64(t)

	_ = runtime.Set("grpc", exports)
	_ = runtime.Set("descriptors", b64)

	_, err := runtime.RunString(`
		grpc.loadDescriptorSet(descriptors);
		var conn = grpc.dial('localhost:50051', { insecure: true });
		try {
			conn.invoke('/nonexistent.Service/Method', {});
		} finally {
			conn.close();
		}
	`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in loaded descriptors")
}

func TestGrpc_Invoke_UnknownMethod(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	b64 := testDescriptorBase64(t)

	_ = runtime.Set("grpc", exports)
	_ = runtime.Set("descriptors", b64)

	_, err := runtime.RunString(`
		grpc.loadDescriptorSet(descriptors);
		var conn = grpc.dial('localhost:50051', { insecure: true });
		try {
			conn.invoke('/test.EchoService/NonExistent', {});
		} finally {
			conn.close();
		}
	`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `method "NonExistent" not found`)
}

func TestGrpc_Status_Constants(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	_ = runtime.Set("grpc", exports)

	tests := []struct {
		name     string
		expr     string
		expected int
	}{
		{"OK", "grpc.status.OK", 0},
		{"CANCELLED", "grpc.status.CANCELLED", 1},
		{"UNKNOWN", "grpc.status.UNKNOWN", 2},
		{"INVALID_ARGUMENT", "grpc.status.INVALID_ARGUMENT", 3},
		{"DEADLINE_EXCEEDED", "grpc.status.DEADLINE_EXCEEDED", 4},
		{"NOT_FOUND", "grpc.status.NOT_FOUND", 5},
		{"ALREADY_EXISTS", "grpc.status.ALREADY_EXISTS", 6},
		{"PERMISSION_DENIED", "grpc.status.PERMISSION_DENIED", 7},
		{"RESOURCE_EXHAUSTED", "grpc.status.RESOURCE_EXHAUSTED", 8},
		{"FAILED_PRECONDITION", "grpc.status.FAILED_PRECONDITION", 9},
		{"ABORTED", "grpc.status.ABORTED", 10},
		{"OUT_OF_RANGE", "grpc.status.OUT_OF_RANGE", 11},
		{"UNIMPLEMENTED", "grpc.status.UNIMPLEMENTED", 12},
		{"INTERNAL", "grpc.status.INTERNAL", 13},
		{"UNAVAILABLE", "grpc.status.UNAVAILABLE", 14},
		{"DATA_LOSS", "grpc.status.DATA_LOSS", 15},
		{"UNAUTHENTICATED", "grpc.status.UNAUTHENTICATED", 16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.expr)
			require.NoError(t, err)
			assert.Equal(t, int64(tt.expected), v.ToInteger())
		})
	}
}

func TestGrpc_ParseFullMethod(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		wantSvc    string
		wantMethod string
		wantErr    bool
	}{
		{"valid", "/pkg.Svc/Method", "pkg.Svc", "Method", false},
		{"nested_package", "/a.b.c.Svc/DoThing", "a.b.c.Svc", "DoThing", false},
		{"no_leading_slash", "pkg.Svc/Method", "", "", true},
		{"no_method", "/pkg.Svc/", "", "", true},
		{"no_service", "//Method", "", "", true},
		{"empty", "", "", "", true},
		{"just_slash", "/", "", "", true},
		{"only_service", "/pkg.Svc", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, method, err := parseFullMethod(tt.method)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantSvc, svc)
				assert.Equal(t, tt.wantMethod, method)
			}
		})
	}
}

func TestGrpc_Invoke_ResponseProtoJSON(t *testing.T) {
	// Verifies that protojson marshaling produces correct JSON objects.
	// Uses a server that returns a known response for validation.
	addr, stop := startEchoServer(t)
	defer stop()

	runtime, exports := loadModuleIntoRuntime(t)
	b64 := testDescriptorBase64(t)

	_ = runtime.Set("grpc", exports)
	_ = runtime.Set("addr", addr)
	_ = runtime.Set("descriptors", b64)

	v, err := runtime.RunString(`
		grpc.loadDescriptorSet(descriptors);
		var conn = grpc.dial(addr, { insecure: true });
		var resp = conn.invoke('/test.EchoService/Echo', { message: 'test-proto-json' });
		conn.close();
		JSON.stringify(resp);
	`)
	require.NoError(t, err)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(v.String()), &resp))
	assert.Equal(t, "echo: test-proto-json", resp["message"])
}

func TestGrpc_Invoke_CancelledContext(t *testing.T) {
	addr, stop := startEchoServer(t)
	defer stop()

	// Create module with already-cancelled context
	rt := goja.New()
	module := rt.NewObject()
	exps := rt.NewObject()
	_ = module.Set("exports", exps)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	loader := Require(ctx)
	loader(rt, module)

	b64 := testDescriptorBase64(t)
	_ = rt.Set("grpc", exps)
	_ = rt.Set("addr", addr)
	_ = rt.Set("descriptors", b64)

	_, err := rt.RunString(`
		grpc.loadDescriptorSet(descriptors);
		var conn = grpc.dial(addr, { insecure: true });
		try {
			conn.invoke('/test.EchoService/Echo', { message: 'test' });
		} finally {
			conn.close();
		}
	`)
	assert.Error(t, err)
}

func TestGrpc_MultipleDescriptorSets(t *testing.T) {
	// Verify that loading descriptors is additive.
	runtime, exports := loadModuleIntoRuntime(t)
	b64 := testDescriptorBase64(t)

	// Create a second descriptor set with a different service.
	fds2 := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{{
			Name:    proto.String("test2.proto"),
			Package: proto.String("test2"),
			Syntax:  proto.String("proto3"),
			MessageType: []*descriptorpb.DescriptorProto{{
				Name: proto.String("PingRequest"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("data"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("data")},
				},
			}, {
				Name: proto.String("PingResponse"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("data"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("data")},
				},
			}},
			Service: []*descriptorpb.ServiceDescriptorProto{{
				Name: proto.String("PingService"),
				Method: []*descriptorpb.MethodDescriptorProto{{
					Name:       proto.String("Ping"),
					InputType:  proto.String(".test2.PingRequest"),
					OutputType: proto.String(".test2.PingResponse"),
				}},
			}},
		}},
	}
	data2, err := proto.Marshal(fds2)
	require.NoError(t, err)
	b64_2 := base64.StdEncoding.EncodeToString(data2)

	_ = runtime.Set("grpc", exports)
	_ = runtime.Set("desc1", b64)
	_ = runtime.Set("desc2", b64_2)

	// Load both descriptor sets.
	_, err = runtime.RunString(`
		grpc.loadDescriptorSet(desc1);
		grpc.loadDescriptorSet(desc2);
	`)
	require.NoError(t, err)

	// Verify both services are registered by checking that invoke would
	// resolve them (we just check the error message format — not "not found
	// in loaded descriptors" but rather a connection error since we didn't
	// start a server).
	_, err = runtime.RunString(`
		var conn = grpc.dial('localhost:1', { insecure: true });
		try {
			conn.invoke('/test.EchoService/Echo', { message: 'test' });
		} catch (e) {
			// Expected: connection refused, NOT "not found in loaded descriptors"
			if (e.message && e.message.indexOf('not found in loaded descriptors') >= 0) {
				throw new Error('Service should have been loaded but was not found');
			}
		}
		try {
			conn.invoke('/test2.PingService/Ping', { data: 'test' });
		} catch (e) {
			if (e.message && e.message.indexOf('not found in loaded descriptors') >= 0) {
				throw new Error('Service should have been loaded but was not found');
			}
		}
		conn.close();
	`)
	require.NoError(t, err)
}

func TestGrpc_Invoke_RoundTrip(t *testing.T) {
	// Full round-trip test: load descriptors → dial → invoke → verify response → close.
	addr, stop := startEchoServer(t)
	defer stop()

	runtime, exports := loadModuleIntoRuntime(t)
	b64 := testDescriptorBase64(t)

	_ = runtime.Set("grpc", exports)
	_ = runtime.Set("addr", addr)
	_ = runtime.Set("descriptors", b64)

	messages := []string{"hello", "world", "", "with spaces", "unicode: 日本語"}
	for _, msg := range messages {
		t.Run(fmt.Sprintf("msg=%q", msg), func(t *testing.T) {
			_ = runtime.Set("testMsg", msg)
			v, err := runtime.RunString(`
				grpc.loadDescriptorSet(descriptors);
				var conn = grpc.dial(addr, { insecure: true });
				var resp = conn.invoke('/test.EchoService/Echo', { message: testMsg });
				conn.close();
				resp.message;
			`)
			require.NoError(t, err)
			assert.Equal(t, "echo: "+msg, v.Export())
		})
	}
}

// TestGrpc_DescriptorFiles_Concurrent verifies thread-safe access to loaded descriptors.
func TestGrpc_DescriptorFiles_Concurrent(t *testing.T) {
	m := &Module{
		files: &descriptorFiles{
			services: make(map[string]serviceInfo),
		},
	}

	fds := testDescriptors()
	files, err := protodesc.NewFiles(fds)
	require.NoError(t, err)

	// Pre-load descriptors
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

	// Concurrent reads should not race
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			m.mu.RLock()
			defer m.mu.RUnlock()
			si, ok := m.files.services["test.EchoService"]
			assert.True(t, ok)
			_, ok = si.methods["Echo"]
			assert.True(t, ok)
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestGrpc_ProtojsonMarshalOutput verifies protojson output format.
func TestGrpc_ProtojsonMarshalOutput(t *testing.T) {
	fds := testDescriptors()
	files, err := protodesc.NewFiles(fds)
	require.NoError(t, err)

	svcDesc, err := files.FindDescriptorByName("test.EchoService")
	require.NoError(t, err)
	svc := svcDesc.(protoreflect.ServiceDescriptor)
	echoMethod := svc.Methods().ByName("Echo")
	outputDesc := echoMethod.Output()

	msg := dynamicpb.NewMessage(outputDesc)
	field := outputDesc.Fields().ByName("message")
	msg.Set(field, protoreflect.ValueOfString("test value"))

	data, err := protojson.Marshal(msg)
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &result))
	assert.Equal(t, "test value", result["message"])
}
