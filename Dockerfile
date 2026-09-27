FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS go-builder

ARG TARGETOS
ARG TARGETARCH
ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}

WORKDIR /usr/src/app

RUN apk add --update \
        curl \
        gcc \
        git \
        make \
        musl-dev \
        file

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
ARG VERSION=dev
RUN make all VERSION=${VERSION}
RUN cp build/web /notmanytask
RUN file /notmanytask build/nmt


# Course tooling for CI jobs: `docker build --target cli`. No entrypoint, so
# any CI can run a shell script in it; git is required by `nmt publish`.
FROM alpine:3.20 AS cli

RUN apk add --no-cache \
        ca-certificates \
        git

COPY --from=go-builder /usr/src/app/build/nmt /usr/local/bin/nmt


# The server, the default target
FROM alpine:3.13

# Install packages required by the image
RUN apk add --update \
        bash \
        ca-certificates \
        coreutils \
        curl \
        jq \
        openssl \
        tzdata \
    && rm /var/cache/apk/*

COPY --from=go-builder /notmanytask /

ENTRYPOINT ["/notmanytask"]
CMD ["-config", "/etc/notmanytask/config.yml"]
