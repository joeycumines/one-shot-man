module github.com/joeycumines/one-shot-man

go 1.27.0

tool (
	github.com/joeycumines/simple-command-output-filter
	golang.org/x/tools/cmd/deadcode
	golang.org/x/tools/cmd/godoc
	honnef.co/go/tools/cmd/staticcheck
	sigs.k8s.io/controller-tools/cmd/controller-gen
)

require (
	charm.land/bubbles/v2 v2.2.1
	charm.land/bubbletea/v2 v2.0.9
	charm.land/lipgloss/v2 v2.0.6
	github.com/creack/pty v1.1.24
	github.com/dlclark/regexp2 v1.12.0
	github.com/expr-lang/expr v1.17.8
	github.com/go-git/go-git/v6 v6.0.0-alpha.5
	github.com/goccy/go-yaml v1.19.2
	github.com/google/uuid v1.6.0
	github.com/joeycumines/go-behaviortree v1.11.0
	github.com/joeycumines/go-eventloop v0.1.0
	github.com/joeycumines/go-inprocgrpc v0.0.0-20260907140835-d1136bc1fe16
	github.com/joeycumines/go-pabt v0.2.0
	github.com/joeycumines/go-prompt v0.0.0-20260907140655-6ebe00a249e2
	github.com/joeycumines/goja v0.0.0-20260825080301-3790b08373a0
	github.com/joeycumines/goja-eventloop v0.1.0
	github.com/joeycumines/goja-grpc v0.1.0
	github.com/joeycumines/goja-protobuf v0.0.0-20260825080318-b4a90e867ce4
	github.com/joeycumines/goja_nodejs v0.0.0-20260825080329-b83bd892783d
	github.com/joeycumines/logiface v0.6.0
	github.com/lrstanley/bubblezone/v2 v2.0.0
	github.com/modelcontextprotocol/go-sdk v1.7.0
	github.com/rivo/uniseg v0.4.7
	github.com/stretchr/testify v1.12.1
	golang.org/x/crypto v0.56.0
	golang.org/x/sys v0.47.0
	golang.org/x/term v0.45.0
	golang.org/x/text v0.41.0
	golang.org/x/tools v0.49.0
	google.golang.org/protobuf v1.36.12
	k8s.io/apimachinery v0.37.0
)

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/ProtonMail/go-crypto v1.4.1 // indirect
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/charmbracelet/colorprofile v0.4.3 // indirect
	github.com/charmbracelet/ultraviolet v0.0.0-20260812204455-68fa937c71be // indirect
	github.com/charmbracelet/x/ansi v0.11.8 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/charmbracelet/x/termios v0.1.1 // indirect
	github.com/charmbracelet/x/windows v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/cloudflare/circl v1.6.5 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dlclark/regexp2/v2 v2.7.2 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/fatih/color v1.19.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.1 // indirect
	github.com/go-git/gcfg/v2 v2.0.2 // indirect
	github.com/go-git/go-billy/v6 v6.0.0-alpha.2 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-openapi/jsonpointer v1.0.0 // indirect
	github.com/go-openapi/jsonreference v1.0.0 // indirect
	github.com/go-openapi/swag v0.27.1 // indirect
	github.com/go-openapi/swag/cmdutils v0.27.1 // indirect
	github.com/go-openapi/swag/conv v0.27.1 // indirect
	github.com/go-openapi/swag/fileutils v0.27.1 // indirect
	github.com/go-openapi/swag/jsonutils v0.27.1 // indirect
	github.com/go-openapi/swag/loading v0.27.1 // indirect
	github.com/go-openapi/swag/mangling v0.27.1 // indirect
	github.com/go-openapi/swag/netutils v0.27.1 // indirect
	github.com/go-openapi/swag/pools v0.27.1 // indirect
	github.com/go-openapi/swag/stringutils v0.27.1 // indirect
	github.com/go-openapi/swag/typeutils v0.27.1 // indirect
	github.com/go-openapi/swag/yamlutils v0.27.1 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/gobuffalo/flect v1.0.3 // indirect
	github.com/google/gnostic-models v0.7.1 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/google/pprof v0.0.0-20260906184651-6331bc6350fe // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/joeycumines/go-bigbuff v1.21.1 // indirect
	github.com/joeycumines/go-catrate v0.0.0-20260825080149-21d1636aba2d // indirect
	github.com/joeycumines/goroutineid v1.1.1 // indirect
	github.com/joeycumines/simple-command-output-filter v0.2.1 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kevinburke/ssh_config v1.6.0 // indirect
	github.com/klauspost/cpuid/v2 v2.4.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.1 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mattn/go-runewidth v0.0.29 // indirect
	github.com/mattn/go-tty v0.0.8 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/pjbgf/sha1cd v0.6.0 // indirect
	github.com/pkg/term v1.2.0-beta.2 // indirect
	github.com/segmentio/asm v1.2.1 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/sergi/go-diff v1.4.0 // indirect
	github.com/spf13/cobra v1.10.2 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xo/terminfo v1.0.0 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	github.com/yuin/goldmark v1.8.6 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/exp v0.0.0-20260824195058-e88cd73687aa // indirect
	golang.org/x/exp/typeparams v0.0.0-20260824195058-e88cd73687aa // indirect
	golang.org/x/mod v0.40.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/telemetry v0.0.0-20260902144106-3ef544be8421 // indirect
	golang.org/x/time v0.15.0 // indirect
	golang.org/x/tools/cmd/godoc v0.1.0-deprecated // indirect
	golang.org/x/tools/godoc v0.1.0-deprecated // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260904194346-d0f1323225a4 // indirect
	google.golang.org/grpc v1.83.2 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	honnef.co/go/tools v0.8.1 // indirect
	k8s.io/api v0.37.0 // indirect
	k8s.io/apiextensions-apiserver v0.37.0 // indirect
	k8s.io/code-generator v0.37.0 // indirect
	k8s.io/gengo/v2 v2.0.0-20260408192533-25e2208e0dc3 // indirect
	k8s.io/klog/v2 v2.140.0 // indirect
	k8s.io/kube-openapi v0.0.0-20260721132016-d427ff9ee9ad // indirect
	k8s.io/utils v0.0.0-20260626114624-be93311217bd // indirect
	sigs.k8s.io/controller-tools v0.22.0 // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.4.2 // indirect
	sigs.k8s.io/yaml v1.6.0 // indirect
)
