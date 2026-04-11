all: web

protos:
	true

make_build:
	mkdir -p build

get_statik:
	go install github.com/rakyll/statik@latest

statik: get_statik protos
	statik -f -Z -src web -dest pkg

web: make_build statik
	GOOS=linux GOARCH=amd64 go build -o build ./cmd/web

crashme: make_build
	go build -o build ./cmd/crashme

all: web

run_web: web
	./build/web

coverage_html:
	go test ./... -coverprofile coverage.out
	go tool cover -html=coverage.out -o coverage.html
	rm coverage.out

docker_image:
	docker build . -f Dockerfile -t bigredeye/notmanytask-base:latest --platform=linux/amd64

docker_hub: docker_image
	docker push bigredeye/notmanytask-base:latest

docker_image_crashme:
	docker build . -f Dockerfile.crashme -t bigredeye/notmanytask:crashme

docker_hub_crashme: docker_image_crashme
	docker push bigredeye/notmanytask:crashme
