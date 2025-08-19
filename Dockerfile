# Build
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS build
WORKDIR /build

# Dependency installation
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Build the app from source
COPY . .
ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o manager cmd/main.go

# Runtime image
FROM gcr.io/distroless/static:nonroot

# Copy only the binary from the build stage to the final image
COPY --from=build /build/manager /

# Set the entry point for the container
ENTRYPOINT ["/manager"]
