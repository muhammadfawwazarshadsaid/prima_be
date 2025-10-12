# Stage 1 - Build
# Menggunakan base image Debian (Bookworm) dengan Go versi 1.25
FROM golang:1.25-bookworm AS builder

WORKDIR /app

# Copy dependency files dan download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy seluruh source code
COPY . .

# Build binary Go
RUN go build -o main .

# Install Python dan pip menggunakan apt-get
RUN apt-get update && apt-get install -y python3 python3-pip

# Copy script dan requirements
COPY script/ /app/script/

# Install dependensi Python
RUN pip3 install --no-cache-dir -r /app/script/requirements.txt --break-system-packages

# Copy model
COPY model/ /app/model/

# Pastikan folder processed_images ada (meski kosong)
RUN mkdir -p /app/processed_images

# Stage 2 - Runtime image ringan (tetap menggunakan Alpine)
FROM alpine:latest
WORKDIR /root/

# Copy binary Go yang sudah di-build dari stage builder
COPY --from=builder /app/main .

# Copy script, model, dan direktori penting lainnya
COPY --from=builder /app/script/ /root/script/
COPY --from=builder /app/model/ /root/model/
COPY --from=builder /app/processed_images /root/processed_images
COPY .env .

EXPOSE 8080
CMD ["./main"]