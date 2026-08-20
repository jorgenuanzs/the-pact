# syntax=docker/dockerfile:1.7

FROM node:24.16.0-alpine3.22@sha256:191c9f0080fcbbc6547a85dc0ff7988072214a355aabdc1d2ec55a7dae5eea8a AS web-dependencies

WORKDIR /src/web

COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci

FROM web-dependencies AS web-source

COPY web/ ./

FROM web-source AS web-test

RUN npm run typecheck \
    && npm test \
    && touch /tmp/pact-web-tests-passed

FROM web-source AS web-build

RUN npm run build

FROM golang:1.26.5-alpine3.24@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS go-source

LABEL io.pact.project="the-pact"

RUN apk add --no-cache build-base git

WORKDIR /src

COPY go.* ./
RUN go mod download

COPY . .

FROM go-source AS source

COPY --from=web-build /src/internal/transport/httpapi/adminui/dist ./internal/transport/httpapi/adminui/dist
COPY --from=web-build /src/internal/transport/httpapi/publicui/dist ./internal/transport/httpapi/publicui/dist

FROM source AS test

ARG GO_TEST_FLAGS

COPY --from=web-test /tmp/pact-web-tests-passed /tmp/pact-web-tests-passed

RUN unformatted="$(find . -type f -name '*.go' -exec gofmt -l {} +)" \
    && test -z "${unformatted}" \
    || { printf 'Unformatted Go files:\n%s\n' "${unformatted}"; exit 1; }

RUN --mount=type=cache,target=/root/.cache/go-build \
    go vet ./...

RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 go test ${GO_TEST_FLAGS} ./...

FROM source AS test-ui

COPY --from=web-test /tmp/pact-web-tests-passed /tmp/pact-web-tests-passed

RUN unformatted="$(find internal/transport/httpapi/adminui internal/transport/httpapi/publicui -type f -name '*.go' -exec gofmt -l {} +)" \
    && test -z "${unformatted}" \
    || { printf 'Unformatted Admin UI Go files:\n%s\n' "${unformatted}"; exit 1; }

RUN --mount=type=cache,target=/root/.cache/go-build \
	go vet ./internal/transport/httpapi/adminui ./internal/transport/httpapi/publicui \
	&& go test ./internal/transport/httpapi/adminui ./internal/transport/httpapi/publicui

FROM source AS build

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build \
      -buildvcs=false \
      -trimpath \
      -ldflags="-s -w \
        -X github.com/jorgenuanzs/the-pact/internal/buildinfo.Version=${VERSION} \
        -X github.com/jorgenuanzs/the-pact/internal/buildinfo.Commit=${COMMIT} \
        -X github.com/jorgenuanzs/the-pact/internal/buildinfo.Date=${BUILD_DATE}" \
      -o /out/pact-server \
      ./cmd/pact-server

FROM go-source AS cli-build

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ARG CLI_GOOS=linux
ARG CLI_GOARCH=arm64

RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${CLI_GOOS} GOARCH=${CLI_GOARCH} go build \
      -buildvcs=false \
      -trimpath \
      -ldflags="-s -w \
        -X github.com/jorgenuanzs/the-pact/internal/buildinfo.Version=${VERSION} \
        -X github.com/jorgenuanzs/the-pact/internal/buildinfo.Commit=${COMMIT} \
        -X github.com/jorgenuanzs/the-pact/internal/buildinfo.Date=${BUILD_DATE}" \
      -o /out/pact \
      ./cmd/pact

FROM scratch AS cli-artifact

COPY --from=cli-build /out/pact /pact

FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS runtime

LABEL io.pact.project="the-pact"

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 pact \
    && adduser -S -D -H -u 10001 -G pact pact

COPY --from=build --chown=pact:pact /out/pact-server /usr/local/bin/pact-server

USER pact
WORKDIR /var/lib/pact

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --quiet --spider http://127.0.0.1:8080/livez || exit 1

ENTRYPOINT ["pact-server"]
CMD ["serve"]
