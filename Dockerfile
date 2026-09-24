FROM golang:1.27 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
# The playwright cli must match the playwright-go version, it installs the driver and firefox.
RUN PWGO_VER=$(go list -m -f '{{.Version}}' github.com/mxschmitt/playwright-go) \
    && CGO_ENABLED=0 go install github.com/mxschmitt/playwright-go/cmd/playwright@${PWGO_VER}
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/firefox-ui-testing-mcp .

FROM ubuntu:noble
ENV PLAYWRIGHT_DRIVER_PATH=/opt/playwright/driver \
    PLAYWRIGHT_BROWSERS_PATH=/opt/playwright/browsers
COPY --from=builder /go/bin/playwright /usr/local/bin/playwright
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata fonts-noto-color-emoji \
    && playwright install --with-deps firefox \
    && rm -rf /var/lib/apt/lists/* /tmp/* \
    && useradd --create-home --uid 10001 browser \
    && chmod -R a+rX /opt/playwright
COPY --from=builder /out/firefox-ui-testing-mcp /usr/local/bin/firefox-ui-testing-mcp
USER browser
ENV PORT=8080
EXPOSE 8080
CMD ["firefox-ui-testing-mcp"]
