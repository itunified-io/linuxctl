# syntax=docker/dockerfile:1.7
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download || true
COPY . .
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.Date=${DATE}" \
    -o /out/linuxctl ./cmd/linuxctl

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=build /out/linuxctl /linuxctl
USER nonroot:nonroot
ENTRYPOINT ["/linuxctl"]
