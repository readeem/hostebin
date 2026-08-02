# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/hostebin ./cmd/hostebin && mkdir /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/hostebin /hostebin
COPY --from=build --chown=65532:65532 /out/data /data
ENV XDG_CONFIG_HOME=/data/config \
    XDG_DATA_HOME=/data
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/hostebin"]
CMD ["serve", "--data", "/data"]
