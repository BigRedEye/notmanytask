all: web crashme

protos:
	true

make_build:
	mkdir -p build

web: make_build
	go build -o build ./cmd/web

crashme: make_build
	CGO_ENABLED=0 go build -ldflags="-extldflags=-static" -o build ./cmd/crashme

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
