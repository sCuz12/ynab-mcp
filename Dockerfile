FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /ynab-mcp .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && addgroup -S app && adduser -S -G app app && mkdir /data && chown app:app /data
COPY --from=build /ynab-mcp /usr/local/bin/ynab-mcp
USER app
ENV MCP_TRANSPORT=http OAUTH_DB_PATH=/data/oauth.db PORT=8080
EXPOSE 8080
VOLUME /data
CMD ["/usr/local/bin/ynab-mcp"]
