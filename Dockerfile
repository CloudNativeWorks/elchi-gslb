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

# Add elchi plugin to CoreDNS plugin.cfg
WORKDIR /build/coredns
RUN echo "elchi:github.com/cloudnativeworks/elchi-gslb" >> plugin.cfg

# Update go.mod to include elchi plugin
RUN go mod edit -replace github.com/cloudnativeworks/elchi-gslb=/build/elchi-gslb
RUN go mod tidy

# Build CoreDNS with elchi plugin
RUN make

# Stage 2: Runtime image
FROM alpine:3.19

# Install ca-certificates for HTTPS
RUN apk add --no-cache ca-certificates

# Copy CoreDNS binary
COPY --from=builder /build/coredns/coredns /usr/bin/coredns

# Copy example Corefile
COPY Corefile.example /etc/coredns/Corefile.example

# Create user for CoreDNS
RUN addgroup -S coredns && adduser -S -G coredns coredns

# Set ownership
RUN chown -R coredns:coredns /etc/coredns

# Expose DNS ports
EXPOSE 53 53/udp
EXPOSE 8053 8053/tcp

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/usr/bin/coredns", "-version"] || exit 1

# Switch to coredns user
USER coredns

# Set entrypoint
ENTRYPOINT ["/usr/bin/coredns"]
CMD ["-conf", "/etc/coredns/Corefile"]
