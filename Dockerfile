## Multi-stage build producing a minimal scratch runtime image.
## Stage 1 (Node) builds the SPA bundle synced into external/ui for go:embed when BUILD_TAGS contains ui.
## Stage 2 (Go) respects BUILD_TAGS (comma-separated, same as make / go build -tags).

FROM node:22-bookworm AS ui-builder

WORKDIR /ui
COPY external/ui/package.json external/ui/package-lock.json ./
RUN npm ci --no-fund --no-audit
COPY external/ui/ ./
COPY docs/assets/foxxycode-logo-mark-flat.svg docs/assets/favicon-32.png docs/assets/favicon.ico docs/assets/apple-touch-icon.png /docs/assets/
RUN npm run build:go


FROM golang:1.25-bookworm AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
# Default build includes the messenger gateway so the image can run `foxxycode gateway`
# by overriding CMD (see docker-compose command override). Pass --build-arg BUILD_TAGS
# to trim it. CI (docker-build-push.yaml) sets its own BUILD_TAGS for the published image.
ARG BUILD_TAGS=http,scheduler,ui,memory,miniapps,gateway,cli,browser,swarm
ARG TARGETOS=linux
ARG TARGETARCH=amd64

ENV CGO_ENABLED=0
ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}
ENV VERSION=${VERSION}
ENV BUILD_TAGS=${BUILD_TAGS}

COPY --from=ui-builder /ui/index.html /ui/styles.css /ui/app.js /src/external/ui/

RUN mkdir -p /out \
	/out/ssl-certs \
	&& GO_TAGS="$(printf '%s' "$BUILD_TAGS" | tr -d '[:space:]')" \
	&& if [ -n "$GO_TAGS" ]; then \
	go build \
	-tags="$GO_TAGS" \
	-trimpath \
	-ldflags "-s -w -X github.com/hijera/foxxycode-agent/internal/version.Version=${VERSION}" \
	-o /out/foxxycode \
	./cmd/foxxycode/; \
	else \
	go build \
	-trimpath \
	-ldflags "-s -w -X github.com/hijera/foxxycode-agent/internal/version.Version=${VERSION}" \
	-o /out/foxxycode \
	./cmd/foxxycode/; \
	fi \
	&& cp /etc/ssl/certs/ca-certificates.crt /out/ssl-certs/ca-certificates.crt


FROM scratch

COPY --from=build /out/foxxycode /bin/foxxycode
COPY --from=build /out/ssl-certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

WORKDIR /workspace

ENV FOXXYCODE_HOME=/home/user/.foxxycode
ENV FOXXYCODE_CWD=/workspace
ENV FOXXYCODE_CONFIG=/home/user/.foxxycode.yaml

EXPOSE 12345

ENTRYPOINT ["/bin/foxxycode"]
# Default subcommand. `serve` starts every subsystem the mounted config.yaml enables -
# the HTTP API and web UI unless it says otherwise, plus the Telegram gateway, the
# swarm relay and the cron scheduler when they are turned on - over one set of
# sessions. The bind address is explicit because a container has to accept
# connections from outside it, where `serve` on its own would answer on loopback.
# Override for a single surface, e.g. `docker run ... gateway --cwd /workspace`, or
# via compose `command:` / the FOXXYCODE_COMMAND override in docker-compose(.dev).yml.
CMD ["serve","-H","0.0.0.0","-P","12345"]
