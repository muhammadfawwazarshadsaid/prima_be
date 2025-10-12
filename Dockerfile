# Stage 1 - Build
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy dependency files dan download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy seluruh source code
COPY . .

# Build binary Go
RUN go build -o main .

# Install Python dan pip
RUN apt-get update && apt-get install -y python3 python3-pip

# Copy script dan requirements
COPY script/ /app/script/

# Install dependensi Python
RUN pip3 install --no-cache-dir -r /app/script/requirements.txt

# Copy model
COPY model/ /app/model/

# Stage 2 - Runtime image ringan
FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/main .
COPY .env .
EXPOSE 8080
CMD ["./main"]
