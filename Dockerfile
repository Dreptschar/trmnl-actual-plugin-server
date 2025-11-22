# ---------- Stage 1: Node dependencies ----------
FROM node:22 AS node-deps

WORKDIR /usr/src/app

# Install Node deps
COPY package*.json ./
RUN npm install

# Copy the rest of the source (JS + Go + whatever)
COPY . .

# ---------- Stage 2: Go build ----------
FROM golang:1.23 AS go-builder

WORKDIR /usr/src/app

# Go modules
COPY go.mod ./
RUN go mod download

# Copy everything (including main.go)
COPY . .

# Build Go binary from main.go
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o server main.go

# ---------- Stage 3: Final image with Node + Go ----------
FROM node:22

WORKDIR /usr/src/app

# Bring in Node app + node_modules + JS files
COPY --from=node-deps /usr/src/app . 

# Bring in Go server binary
COPY --from=go-builder /usr/src/app/server /usr/local/bin/server

# Go HTTP server listening port
EXPOSE 8080

# Start the Go server (which calls `node script.js` internally)
CMD ["server"]
