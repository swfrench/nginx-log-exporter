# Example: native build in a minimal nonroot-flavored distroless image.

FROM docker.io/library/golang:1.21-bookworm as builder

WORKDIR /build

COPY . .

RUN go mod download && go mod verify
RUN CGO_ENABLED=0 go build -o nginx-log-exporter

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /build/nginx-log-exporter /
