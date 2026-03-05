# Frontend build stage
FROM node:22-alpine AS frontend

WORKDIR /app

COPY frontend/package.json frontend/package-lock.json ./
RUN npm install

COPY frontend/ .
RUN npm run build

# Go build stage
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Copy Go module files
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Copy source
COPY backend/ .

# Copy built frontend into public/ for go:embed
COPY --from=frontend /backend/public/ ./public/

# Build
RUN CGO_ENABLED=0 go build -o /passedbox-server .

# Runtime stage
FROM alpine:3.21

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /passedbox-server .

# Create data directory
RUN mkdir -p /app/data

ENV DATA_DIR=/app/data
ENV PORT=8085
ENV WORKER_INTERVAL=12h

EXPOSE 8085

CMD ["./passedbox-server"]
