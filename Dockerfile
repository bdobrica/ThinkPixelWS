FROM golang:1.25.14-trixie@sha256:2c4c60ef415fbfa5e90300722293bef36c5e63fae17570ce18f580af933dbd73 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w -buildid=' -o /out/thinkpixelws ./cmd/thinkpixelws

FROM gcr.io/distroless/static-debian13:nonroot@sha256:1c2c046bc09ed40fad370b599a0b1ae7987f55b01e247cf27a7c27cd97e5bbc7

LABEL org.opencontainers.image.source="https://github.com/bdobrica/ThinkPixelWS" \
      org.opencontainers.image.title="ThinkPixelWS" \
      org.opencontainers.image.licenses="Apache-2.0"

COPY --from=build --chown=65532:65532 /out/thinkpixelws /usr/local/bin/thinkpixelws
USER 65532:65532
WORKDIR /home/nonroot
EXPOSE 8080 9090
ENTRYPOINT ["/usr/local/bin/thinkpixelws"]
