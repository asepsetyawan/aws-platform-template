FROM golang:1.27.2-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG SERVICE=api
RUN case "$SERVICE" in api|catalog) ;; *) exit 2 ;; esac && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server "./cmd/$SERVICE"

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/server /server
USER 10001:10001
EXPOSE 8080 9090 50051
ENTRYPOINT ["/server"]
