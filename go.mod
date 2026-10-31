module github.com/joeycumines/one-shot-man

go 1.27.3

tool (
	github.com/joeycumines/simple-command-output-filter
	golang.org/x/tools/cmd/deadcode
	golang.org/x/tools/cmd/godoc
	honnef.co/go/tools/cmd/staticcheck
	sigs.k8s.io/controller-tools/cmd/controller-gen
)

require (
	charm.land/bubbles/v2 v2.2.1
	charm.land/bubbletea/v2 v2.1.1
	charm.land/lipgloss/v2 v2.0.6
	github.com/charmbracelet/ultraviolet v0.0.0-20261023060817-a45b7c86333f
	github.com/creack/pty v1.1.24
	github.com/dlclark/regexp2 v1.12.0
	github.com/expr-lang/expr v1.17.8
	github.com/go-git/go-git/v6 v6.0.0-beta.1
	github.com/goccy/go-yaml v1.19.2
	github.com/google/uuid v1.6.0
	github.com/joeycumines/go-behaviortree v1.11.0
	github.com/joeycumines/go-bigbuff v1.21.1
	github.com/joeycumines/go-eventloop v0.1.0
	github.com/joeycumines/go-inprocgrpc v0.1.0
	github.com/joeycumines/go-pabt v0.2.0
	github.com/joeycumines/go-prompt v0.0.0-20261011012353-082420ebf41c
	github.com/joeycumines/goja v0.0.0-20261011012432-cfeebc479e4c
	github.com/joeycumines/goja-eventloop v0.2.0
	github.com/joeycumines/goja-grpc v0.1.0
	github.com/joeycumines/goja-protobuf v0.0.0-20261011012501-c2c0a1db0aa6
	github.com/joeycumines/goja_nodejs v0.0.0-20261011012520-94f19c9a65bb
	github.com/joeycumines/logiface v0.7.0
	github.com/lrstanley/bubblezone/v2 v2.0.0
	github.com/modelcontextprotocol/go-sdk v1.8.0
	github.com/rivo/uniseg v0.4.7
	github.com/stretchr/testify v1.12.1
	golang.org/x/crypto v0.58.0
	golang.org/x/sys v0.49.0
	golang.org/x/term v0.47.0
	golang.org/x/text v0.43.0
	golang.org/x/tools v0.52.0
	google.golang.org/protobuf v1.36.12
	k8s.io/apiextensions-apiserver v0.38.0-alpha.3
	k8s.io/apimachinery v0.38.0-alpha.3
	k8s.io/client-go v0.38.0-alpha.3
	sigs.k8s.io/yaml v1.6.0
)

require (
	cel.dev/expr v0.25.3 // indirect
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/Microsoft/go-winio v0.6.4 // indirect
	github.com/ProtonMail/go-crypto v1.5.2 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/charmbracelet/colorprofile v0.4.3 // indirect
	github.com/charmbracelet/x/ansi v0.11.9 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/charmbracelet/x/termios v0.1.1 // indirect
	github.com/charmbracelet/x/windows v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/cloudflare/circl v1.6.5 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dlclark/regexp2/v2 v2.12.0 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/fatih/color v1.19.0 // indirect
	github.com/fxamacker/cbor/v2 v2.10.0 // indirect
	github.com/go-git/gcfg/v2 v2.0.2 // indirect
	github.com/go-git/go-billy/v6 v6.0.0-beta.1 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-openapi/jsonpointer v1.0.2 // indirect
	github.com/go-openapi/jsonreference v1.0.3 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/gobuffalo/flect v1.0.3 // indirect
	github.com/google/cel-go v0.31.0 // indirect
	github.com/google/gnostic-models v0.7.1 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/google/pprof v0.0.0-20261026192309-dbd0ed30aa88 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/joeycumines/go-catrate v0.0.0-20261011012218-29b87efa7627 // indirect
	github.com/joeycumines/goroutineid v1.1.1 // indirect
	github.com/joeycumines/simple-command-output-filter v0.2.1 // indirect
	github.com/kevinburke/ssh_config v1.6.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.1 // indirect
	github.com/mattn/go-colorable v0.1.16 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mattn/go-runewidth v0.0.31 // indirect
	github.com/mattn/go-tty v0.0.8 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pandatix/go-abnf v0.5.4 // indirect
	github.com/pjbgf/sha1cd v0.7.0 // indirect
	github.com/pkg/term v1.2.0-beta.2 // indirect
	github.com/prometheus/client_golang v1.25.0 // indirect
	github.com/prometheus/client_model v0.6.3 // indirect
	github.com/prometheus/common v0.73.1 // indirect
	github.com/prometheus/procfs v0.23.0 // indirect
	github.com/segmentio/asm v1.2.1 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/sergi/go-diff v1.4.0 // indirect
	github.com/spf13/cobra v1.10.2 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xo/terminfo v1.3.0 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	github.com/yuin/goldmark v1.8.7 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/exp v0.0.0-20261014143641-f0251eb4aa3e // indirect
	golang.org/x/exp/typeparams v0.0.0-20261014143641-f0251eb4aa3e // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/oauth2 v0.37.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/telemetry v0.0.0-20261021172232-6cd571d43fbb // indirect
	golang.org/x/time v0.16.0 // indirect
	golang.org/x/tools/cmd/godoc v0.1.0-deprecated // indirect
	golang.org/x/tools/godoc v0.1.0-deprecated // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20261026165753-2ffab32bce7e // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261026165753-2ffab32bce7e // indirect
	google.golang.org/grpc v1.85.0 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.13.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	honnef.co/go/tools v0.8.2 // indirect
	k8s.io/api v0.38.0-alpha.3 // indirect
	k8s.io/apiserver v0.38.0-alpha.3 // indirect
	k8s.io/code-generator v0.38.0-alpha.3 // indirect
	k8s.io/component-base v0.38.0-alpha.3 // indirect
	k8s.io/gengo/v2 v2.0.0-20260408192533-25e2208e0dc3 // indirect
	k8s.io/klog/v2 v2.140.0 // indirect
	k8s.io/kube-openapi v0.0.0-20261014044854-ca3452a79603 // indirect
	k8s.io/utils v0.0.0-20261023123148-ca04ba58ecab // indirect
	sigs.k8s.io/controller-tools v0.23.0 // indirect
	sigs.k8s.io/json v0.0.0-20260909141634-11ed52e25bc5 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v7 v7.0.1 // indirect
)

replace github.com/muesli/cancelreader => github.com/joeycumines/go-cancelreader v0.0.0-20261006054419-50bbc8357f95
