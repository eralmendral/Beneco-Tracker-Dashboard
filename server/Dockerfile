FROM golang:1.26-alpine AS build
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/beneco-server ./cmd/server

FROM python:3.13-alpine
RUN addgroup -S app && adduser -S -G app app
COPY scripts/requirements.txt /app/scripts/requirements.txt
RUN pip install --no-cache-dir -r /app/scripts/requirements.txt
COPY scripts/scrape_beneco.py scripts/pdf_cells.py /app/scripts/
COPY --from=build /out/beneco-server /usr/local/bin/beneco-server
USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/beneco-server"]
