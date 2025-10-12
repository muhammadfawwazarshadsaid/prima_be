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

# Install Python dan pip menggunakan apk
RUN apk update && apk add --no-cache python3 py3-pip

# Copy script dan requirements
COPY script/ /app/script/

# Install dependensi Python <--- PERUBAHAN DI SINI
RUN pip3 install --no-cache-dir -r /app/script/requirements.txt --break-system-packages

# Copy model
COPY model/ /app/model/

# Stage 2 - Runtime image ringan
FROM alpine:latest
WORKDIR /root/

# Copy binary Go yang sudah di-build dari stage builder
COPY --from=builder /app/main .

# Copy script, model, dan direktori gambar yang diproses
COPY --from=builder /app/script/ /root/script/
COPY --from=builder /app/model/ /root/model/
COPY --from=builder /app/processed_images /root/processed_images
COPY .env .

EXPOSE 8080
CMD ["./main"]