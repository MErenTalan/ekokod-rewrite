# syntax=docker/dockerfile:1

FROM golang:1.27.1-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates tzdata
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build \
      -ldflags "-s -w \
        -X github.com/MErenTalan/ekokod-rewrite/internal/buildinfo.version=${VERSION} \
        -X github.com/MErenTalan/ekokod-rewrite/internal/buildinfo.commit=${COMMIT} \
        -X github.com/MErenTalan/ekokod-rewrite/internal/buildinfo.date=${DATE}" \
      -o /out/ekokod ./cmd/ekokod

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata wget \
 && adduser -D -u 10001 ekokod \
 && mkdir -p /var/lib/ekokod && chown ekokod:ekokod /var/lib/ekokod
ENV TZ=Europe/Istanbul
COPY --from=build /out/ekokod /usr/local/bin/ekokod
USER ekokod
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/ekokod"]
CMD ["api"]
