FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
      -ldflags "-s -w -X github.com/readeem/hostebin/internal/version.Version=${VERSION#v}" \
      -o /out/hostebin ./cmd/hostebin \
    && mkdir /out/data

FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.source=https://github.com/readeem/hostebin \
      org.opencontainers.image.description="Upload a bundle of files and get a shareable web URL" \
      org.opencontainers.image.licenses=MIT

COPY --from=build /out/hostebin /hostebin
COPY --from=build --chown=65532:65532 /out/data /data

ENV XDG_CONFIG_HOME=/data/config \
    XDG_DATA_HOME=/data

VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/hostebin"]
CMD ["serve", "--data", "/data"]
