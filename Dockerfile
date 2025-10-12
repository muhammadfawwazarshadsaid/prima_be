# ===========================
# 🏗️ Stage 1 - Build
# ===========================
FROM golang:latest AS builder

WORKDIR /app

# Install Python dan pip
RUN apt-get update && apt-get install -y python3 python3-pip libgl1 libglib2.0-0

# Install Go 1.25.1 toolchain (just in case)
RUN go install golang.org/dl/go1.25.1@latest && \
    /root/go/bin/go1.25.1 download && \
    export GOTOOLCHAIN=go1.25.1

# Copy dependency files dan download Go dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy seluruh source code
COPY . .

# Build binary Go
RUN go build -o main .

# Copy script dan install Python dependencies
COPY script/ /app/script/
RUN pip3 install --no-cache-dir -r /app/script/requirements.txt

# Copy model
COPY model/ /app/model/

# ===========================
# 🚀 Stage 2 - Runtime Image
# ===========================
FROM debian:bullseye-slim
WORKDIR /root/
COPY --from=builder /app/main .
COPY --from=builder /app/script /root/script
COPY --from=builder /app/model /root/model
COPY .env .

EXPOSE 8080
CMD ["./main"]
