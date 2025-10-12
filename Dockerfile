# Stage 1 - Build
# Menggunakan base image Debian (Bookworm) dengan Go versi 1.25
FROM golang:1.25-bookworm AS builder

WORKDIR /app

# Optimasi Cache: Install dependensi Python dulu
RUN apt-get update && apt-get install -y python3 python3-pip
COPY script/requirements.txt /app/script/requirements.txt
RUN pip3 install --no-cache-dir -r /app/script/requirements.txt --break-system-packages

# Optimasi Cache: Install dependensi Go
COPY go.mod go.sum ./
RUN go mod download

# Copy seluruh kode aplikasi dan build static binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# ====================================================================
# Stage 2 - Runtime image ringan (tetap menggunakan Alpine)
# ====================================================================
FROM alpine:latest
WORKDIR /root/

# ====================================================================
# INI PERBAIKANNYA: Install Python di dalam final image
# ====================================================================
RUN apk update && apk add --no-cache python3

# Copy binary Go yang sudah di-build dari stage builder
COPY --from=builder /app/main .

# Copy script, model, dan .env
COPY --from=builder /app/script/ /root/script/
COPY --from=builder /app/model/ /root/model/
COPY .env .

EXPOSE 8080
CMD ["./main"]