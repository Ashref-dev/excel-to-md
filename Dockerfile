# syntax=docker/dockerfile:1
FROM golang:1.27.1-alpine AS builder

RUN apk add --no-cache ca-certificates git

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/excel-to-md ./cmd/server

FROM gcr.io/distroless/static-debian13

WORKDIR /app
COPY --from=builder /out/excel-to-md /app/excel-to-md

ENV ADDR=:8080
EXPOSE 8080

USER nonroot:nonroot
ENTRYPOINT ["/app/excel-to-md"]
