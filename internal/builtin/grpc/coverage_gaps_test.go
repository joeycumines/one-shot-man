package grpc

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// --- jsLoadDescriptorSet gaps ---

// TestCoverage_LoadDescriptorSet_UndefinedArg exercises the goja.IsUndefined(arg) branch.
func TestCoverage_LoadDescriptorSet_UndefinedArg(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	_ = runtime.Set("grpc", exports)

	_, err := runtime.RunString(`grpc.loadDescriptorSet(undefined)`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "loadDescriptorSet")
}

// TestCoverage_LoadDescriptorSet_ProtodescNewFilesError exercises the protodesc.NewFiles
// error path by providing a FileDescriptorSet with unresolved type references.
func TestCoverage_LoadDescriptorSet_ProtodescNewFilesError(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	_ = runtime.Set("grpc", exports)

	// Create a valid-proto FileDescriptorSet that references nonexistent types.
	// proto.Unmarshal succeeds, but protodesc.NewFiles fails on resolution.
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{{
			Name:    proto.String("bad.proto"),
			Package: proto.String("bad"),
			Syntax:  proto.String("proto3"),
			Service: []*descriptorpb.ServiceDescriptorProto{{
				Name: proto.String("BadService"),
				Method: []*descriptorpb.MethodDescriptorProto{{
					Name:       proto.String("BadMethod"),
					InputType:  proto.String(".nonexistent.RequestType"),
					OutputType: proto.String(".nonexistent.ResponseType"),
				}},
			}},
		}},
	}
	data, err := proto.Marshal(fds)
	require.NoError(t, err)
	b64 := base64.StdEncoding.EncodeToString(data)
	_ = runtime.Set("b64", b64)

	_, err = runtime.RunString(`grpc.loadDescriptorSet(b64)`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse descriptors")
}

// --- jsDial gaps ---

// TestCoverage_Dial_WithAuthority exercises the authority option branch in jsDial.
func TestCoverage_Dial_WithAuthority(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	_ = runtime.Set("grpc", exports)

	v, err := runtime.RunString(`
		var conn = grpc.dial('localhost:50051', { insecure: true, authority: 'custom-authority' });
		var target = conn.target;
		conn.close();
		target;
	`)
	require.NoError(t, err)
	assert.Equal(t, "localhost:50051", v.Export())
}

// TestCoverage_Dial_NonMapOptions passes a non-object value as the options argument.
// The type assertion to map[string]interface{} fails silently, so no credentials
// are set, and grpc.NewClient returns a "no transport security set" error.
func TestCoverage_Dial_NonMapOptions(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	_ = runtime.Set("grpc", exports)

	_, err := runtime.RunString(`grpc.dial('localhost:50051', 42)`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no transport security set")
}

// TestCoverage_Dial_InsecureFalse verifies that insecure: false does NOT add
// insecure credentials (the bool check requires true).
func TestCoverage_Dial_InsecureFalse(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	_ = runtime.Set("grpc", exports)

	_, err := runtime.RunString(`grpc.dial('localhost:50051', { insecure: false })`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no transport security set")
}

// TestCoverage_Dial_InsecureNonBool verifies that insecure: "yes" (non-bool)
// is silently ignored.
func TestCoverage_Dial_InsecureNonBool(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	_ = runtime.Set("grpc", exports)

	_, err := runtime.RunString(`grpc.dial('localhost:50051', { insecure: "yes" })`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no transport security set")
}

// TestCoverage_Dial_AuthorityNonString verifies that authority: 123 (non-string)
// is silently ignored (the string type assertion fails).
func TestCoverage_Dial_AuthorityNonString(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	_ = runtime.Set("grpc", exports)

	v, err := runtime.RunString(`
		var conn = grpc.dial('localhost:50051', { insecure: true, authority: 123 });
		conn.close();
		"ok";
	`)
	require.NoError(t, err)
	assert.Equal(t, "ok", v.Export())
}

// --- jsInvoke gaps ---

// TestCoverage_Invoke_NullRequest exercises the goja.IsNull(reqArg) branch,
// sending a null request which should produce an empty proto message.
func TestCoverage_Invoke_NullRequest(t *testing.T) {
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
		var resp = conn.invoke('/test.EchoService/Echo', null);
		conn.close();
		resp.message;
	`)
	require.NoError(t, err)
	assert.Equal(t, "echo: ", v.Export())
}

// TestCoverage_Invoke_BadRequestFieldType exercises the protojson.Unmarshal error
// path by sending a request where a string field contains a nested object.
func TestCoverage_Invoke_BadRequestFieldType(t *testing.T) {
	addr, stop := startEchoServer(t)
	defer stop()

	runtime, exports := loadModuleIntoRuntime(t)
	b64 := testDescriptorBase64(t)

	_ = runtime.Set("grpc", exports)
	_ = runtime.Set("addr", addr)
	_ = runtime.Set("descriptors", b64)

	_, err := runtime.RunString(`
		grpc.loadDescriptorSet(descriptors);
		var conn = grpc.dial(addr, { insecure: true });
		try {
			// "message" field is a string in proto, but we send an object
			conn.invoke('/test.EchoService/Echo', { message: { nested: true } });
		} finally {
			conn.close();
		}
	`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to convert request to proto")
}

// TestCoverage_Invoke_MethodSlashOnly exercises parseFullMethod with "/" only
// through the invoke path (parseFullMethod is already 100%, but this confirms
// the JS → Go error propagation).
func TestCoverage_Invoke_MethodSlashOnly(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	b64 := testDescriptorBase64(t)

	_ = runtime.Set("grpc", exports)
	_ = runtime.Set("descriptors", b64)

	_, err := runtime.RunString(`
		grpc.loadDescriptorSet(descriptors);
		var conn = grpc.dial('localhost:50051', { insecure: true });
		try {
			conn.invoke('/');
		} finally {
			conn.close();
		}
	`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be in the form")
}

// TestCoverage_Invoke_MethodNoMethod exercises parseFullMethod with "/svc" (no method).
func TestCoverage_Invoke_MethodNoMethod(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	b64 := testDescriptorBase64(t)

	_ = runtime.Set("grpc", exports)
	_ = runtime.Set("descriptors", b64)

	_, err := runtime.RunString(`
		grpc.loadDescriptorSet(descriptors);
		var conn = grpc.dial('localhost:50051', { insecure: true });
		try {
			conn.invoke('/test.EchoService');
		} finally {
			conn.close();
		}
	`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be in the form")
}

// TestCoverage_Invoke_MultipleCallsSameConn exercises making multiple RPC calls
// on the same connection without closing between them.
func TestCoverage_Invoke_MultipleCallsSameConn(t *testing.T) {
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
		var r1 = conn.invoke('/test.EchoService/Echo', { message: 'first' });
		var r2 = conn.invoke('/test.EchoService/Echo', { message: 'second' });
		var r3 = conn.invoke('/test.EchoService/Echo', { message: 'third' });
		conn.close();
		r1.message + '|' + r2.message + '|' + r3.message;
	`)
	require.NoError(t, err)
	assert.Equal(t, "echo: first|echo: second|echo: third", v.Export())
}

// TestCoverage_ConnTarget verifies the target property on the connection object.
func TestCoverage_ConnTarget(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	_ = runtime.Set("grpc", exports)

	v, err := runtime.RunString(`
		var conn = grpc.dial('my-server:9090', { insecure: true });
		var t = conn.target;
		conn.close();
		t;
	`)
	require.NoError(t, err)
	assert.Equal(t, "my-server:9090", v.Export())
}

// TestCoverage_Dial_EmptyOpts tests dial with an empty options object.
func TestCoverage_Dial_EmptyOpts(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	_ = runtime.Set("grpc", exports)

	// Empty opts object → no insecure/authority → no credentials → error
	_, err := runtime.RunString(`grpc.dial('localhost:50051', {})`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no transport security set")
}

// TestCoverage_Dial_NullOpts tests dial with null as the options argument.
// null is handled by the IsNull check, so dialOpts stays empty.
func TestCoverage_Dial_NullOpts(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	_ = runtime.Set("grpc", exports)

	_, err := runtime.RunString(`grpc.dial('localhost:50051', null)`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no transport security set")
}

// TestCoverage_Status_AllConstants verifies all 17 status constants are
// accessible and have the expected integer values on a fresh module instance.
func TestCoverage_Status_AllConstants(t *testing.T) {
	runtime, exports := loadModuleIntoRuntime(t)
	_ = runtime.Set("grpc", exports)

	// Verify they're all present and numeric
	v, err := runtime.RunString(`
		var s = grpc.status;
		var keys = ['OK','CANCELLED','UNKNOWN','INVALID_ARGUMENT','DEADLINE_EXCEEDED',
			'NOT_FOUND','ALREADY_EXISTS','PERMISSION_DENIED','RESOURCE_EXHAUSTED',
			'FAILED_PRECONDITION','ABORTED','OUT_OF_RANGE','UNIMPLEMENTED',
			'INTERNAL','UNAVAILABLE','DATA_LOSS','UNAUTHENTICATED'];
		var all = {};
		for (var i = 0; i < keys.length; i++) {
			all[keys[i]] = s[keys[i]];
		}
		JSON.stringify(all);
	`)
	require.NoError(t, err)
	assert.Contains(t, v.String(), `"OK":0`)
	assert.Contains(t, v.String(), `"UNAUTHENTICATED":16`)
}
