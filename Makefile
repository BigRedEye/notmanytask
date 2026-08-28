all: web crashme

protos:
	true

make_build:
	mkdir -p build

web: make_build
	GOARCH=amd64 GOOS=linux go build -o build ./cmd/web

crashme: make_build
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -ldflags="-extldflags=-static" -o build ./cmd/crashme

run_web: web
	./build/web

coverage_html:
	go test ./... -coverprofile coverage.out
	go tool cover -html=coverage.out -o coverage.html
	rm coverage.out

docker_image:
	docker build . -f Dockerfile -t bigredeye/notmanytask:latest --platform=linux/amd64

docker_hub: docker_image
	docker push bigredeye/notmanytask:latest

docker_image_crashme:
	docker build . -f Dockerfile.crashme -t bigredeye/notmanytask:crashme --platform=linux/amd64

docker_hub_crashme: docker_image_crashme
	docker push bigredeye/notmanytask:crashme

.PHONY: test test-unit test-fake test-unit-verified

test: test-unit

test-unit:
	GOFLAGS= go test -count=1 ./...

test-fake:
	GOFLAGS= go test -count=1 ./internal/testkit/gitlabfake

test-unit-verified:
	@results=$$(mktemp /tmp/notmanytask-test-json.XXXXXX) || exit; \
	trap 'status=$$?; trap - 0; rm -f "$$results"; exit "$$status"' 0; \
	trap 'exit 129' 1; trap 'exit 130' 2; trap 'exit 131' 3; trap 'exit 143' 15; \
	GOFLAGS= go test -count=1 -json ./... >"$$results"; test_status=$$?; \
	if [ "$$test_status" -ne 0 ]; then cat "$$results"; exit "$$test_status"; fi; \
	GOFLAGS= go run ./internal/testkit/cmd/checktestjson -manifest internal/testkit/required-unit-tests.txt <"$$results"
