FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
WORKDIR /app
COPY go.mod go.sum ./
# The root module has a local `replace` to the stdlib-only client module
# (used only by tests); its go.mod must be present for module resolution.
COPY client/go.mod ./client/go.mod
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build \
    -ldflags "-X github.com/francisguimond/kairos/internal/version.Version=${VERSION}" \
    -o kairos ./cmd/kairos

FROM alpine:3.24
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/kairos .
EXPOSE 8080
ENTRYPOINT ["./kairos"]
CMD ["run"]
