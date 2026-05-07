## Multi-stage build producing a minimal scratch runtime image.
## Build includes optional modules via Go build tags.

FROM golang:1.25-bookworm AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev

ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64

RUN go build \
  -tags=http,scheduler \
  -ldflags "-s -w -X github.com/EvilFreelancer/coddy-agent/internal/version.Version=${VERSION}" \
  -o /out/coddy \
  ./cmd/coddy/

RUN mkdir -p /out/ssl-certs && cp /etc/ssl/certs/ca-certificates.crt /out/ssl-certs/ca-certificates.crt

FROM scratch

COPY --from=build /out/coddy /bin/coddy
COPY --from=build /out/ssl-certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

WORKDIR /workspace

ENV CODDY_HOME=/home/user/.coddy
ENV CODDY_CWD=/workspace
ENV CODDY_CONFIG=/home/user/.coddy.yaml

EXPOSE 12345

ENTRYPOINT ["/bin/coddy"]
CMD ["http","-H","0.0.0.0","-P","12345"]

