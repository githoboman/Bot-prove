FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY engine/ .
RUN go mod tidy
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /botprove ./cmd/botprove

FROM scratch
COPY --from=builder /botprove /botprove
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENV API_PORT=10000
EXPOSE 10000
ENTRYPOINT ["/botprove", "serve"]
