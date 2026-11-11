VERSION ?= 5.0.0-dev.3
LDFLAGS := -s -w -X main.Version=$(VERSION)
PLATFORMS := linux/amd64 linux/arm64 linux/ppc64le aix/ppc64 solaris/amd64

.PHONY: build test e2e dist clean
build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o build/sas_lsm ./cmd/sas_lsm
test:
	go vet ./... && go test ./...
e2e: build
	test/e2e.sh build/sas_lsm
dist:
	@mkdir -p dist
	@for p in $(PLATFORMS); do os=$${p%/*}; arch=$${p#*/}; \
	  echo "building $$os/$$arch"; \
	  GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/sas_lsm-$$os-$$arch ./cmd/sas_lsm || exit 1; \
	done
clean:
	rm -rf build dist
