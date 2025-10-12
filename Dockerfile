# Stage 1 - Build
# Menggunakan base image Debian (Bookworm) dengan Go versi 1.25
FROM golang:1.25-bookworm AS builder

WORKDIR /app

# ====================================================================
# LANGKAH 1: Install Dependensi Python (yang paling lambat)
# Layer ini hanya akan di-build ulang jika requirements.txt berubah.
# ====================================================================
RUN apt-get update && apt-get install -y python3 python3-pip
COPY script/requirements.txt /app/script/requirements.txt
RUN pip3 install --no-cache-dir -r /app/script/requirements.txt --break-system-packages

# ====================================================================
# LANGKAH 2: Install Dependensi Go
# Layer ini hanya akan di-build ulang jika go.mod atau go.sum berubah.
# ====================================================================
COPY go.mod go.sum ./
RUN go mod download

# ====================================================================
# LANGKAH 3: Copy Seluruh Kode Aplikasi dan Build
# Hanya bagian ini yang akan sering di-build ulang, tapi prosesnya cepat.
# ====================================================================
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o main .


# Stage 2 - Runtime image ringan (tetap menggunakan Alpine)
FROM alpine:latest
WORKDIR /root/

# Copy binary Go yang sudah di-build dari stage builder
COPY --from=builder /app/main .

# Copy script, model, dan .env
COPY --from=builder /app/script/ /root/script/
COPY --from=builder /app/model/ /root/model/
COPY .env .

EXPOSE 8080
CMD ["./main"]