FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY internal ./internal
COPY cmd ./cmd
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/rendezvous ./cmd/rendezvous

FROM alpine:3.22
COPY --from=build /out/rendezvous /usr/local/bin/rendezvous
RUN addgroup -S -g 10001 linksend && adduser -S -D -H -u 10001 -G linksend linksend && mkdir -p /var/lib/linksend && chown -R linksend:linksend /var/lib/linksend
USER linksend:linksend
EXPOSE 8787
ENTRYPOINT ["/usr/local/bin/rendezvous"]
