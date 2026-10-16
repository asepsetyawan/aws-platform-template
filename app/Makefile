include tools.mk
GO ?= go
PROTO_BIN := $(abspath ../.generated/proto-bin)
.PHONY: test proto build

test:
	$(GO) test -race ./...

build:
	$(GO) build ./cmd/...

proto:
	@test "$$(protoc --version)" = "libprotoc $(PROTOC_VERSION)" || (echo "Install protoc $(PROTOC_VERSION)" && exit 1)
	GOBIN=$(PROTO_BIN) $(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	GOBIN=$(PROTO_BIN) $(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)
	PATH="$(PROTO_BIN):$$PATH" protoc --go_out=. --go_opt=module=example.com/aws-platform/app --go-grpc_out=. --go-grpc_opt=module=example.com/aws-platform/app api/catalog/v1/catalog.proto
