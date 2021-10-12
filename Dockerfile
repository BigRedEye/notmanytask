FROM golang:1.17.2-alpine AS go-builder

WORKDIR /usr/src/app

RUN apk add --update \
        curl \
        gcc \
        git \
        make \
        musl-dev

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN make all
RUN cp build/web /notmanytask


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
