# Multi-stage build for CoreDNS with Elchi plugin

# Stage 1: Build CoreDNS with Elchi plugin
FROM golang:1.25.1-alpine AS builder

ARG COREDNS_VERSION=v1.13.2
ARG TARGETARCH

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /build

# Clone CoreDNS
RUN git clone --depth 1 --branch ${COREDNS_VERSION} https://github.com/coredns/coredns.git

# Copy elchi plugin source
COPY . /build/elchi-gslb/

# Add elchi plugin to CoreDNS plugin.cfg (MUST be before file plugin)
WORKDIR /build/coredns
RUN sed -i '/^file:file/i elchi:github.com/cloudnativeworks/elchi-gslb' plugin.cfg

# Update go.mod to include elchi plugin
RUN go mod edit -replace github.com/cloudnativeworks/elchi-gslb=/build/elchi-gslb
RUN go mod tidy

# Build CoreDNS with elchi plugin
RUN make

# Stage 2: Runtime image
FROM alpine:3.21.3

# Install ca-certificates, openssl, and libcap (for setcap)
RUN apk --no-cache update && apk --no-cache upgrade && \
    apk add --no-cache ca-certificates openssl libcap

# Copy CoreDNS binary
COPY --from=builder /build/coredns/coredns /usr/bin/coredns

# Set capability to allow binding to privileged ports (< 1024) as non-root
# Must be done AFTER COPY because COPY does not preserve extended attributes (xattr)
RUN setcap cap_net_bind_service=+ep /usr/bin/coredns && \
    apk del libcap

# Copy example Corefile
COPY Corefile.example /etc/coredns/Corefile.example

# Create coredns user with specific UID/GID 1000
RUN addgroup -g 1000 -S coredns && adduser -u 1000 -S -G coredns coredns

# Set ownership
RUN chown -R coredns:coredns /etc/coredns

# Expose DNS ports
EXPOSE 53 53/udp
EXPOSE 8053 8053/tcp

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/usr/bin/coredns", "-version"] || exit 1

# Switch to non-root user (UID 1000:1000)
# The binary has cap_net_bind_service capability to bind port 53
USER 1000:1000

# Set entrypoint
ENTRYPOINT ["/usr/bin/coredns"]
CMD ["-conf", "/etc/coredns/Corefile"]
