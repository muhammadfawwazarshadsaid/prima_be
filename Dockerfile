# ===================================================================
# 🧩 STAGE 1: PYTHON DEPENDENCIES (BUILDER LAYER)
#
# Tujuan: Membuat lingkungan Python yang terisolasi (venv)
# berisi semua dependensi yang dibutuhkan.
# ===================================================================
FROM python:3.11-slim-bookworm AS python-base

# Tentukan path untuk virtual environment
ENV VENV_PATH=/opt/venv
# Buat virtual environment
RUN python3 -m venv $VENV_PATH
# Atur PATH agar perintah berikutnya (seperti pip) menggunakan venv
ENV PATH="$VENV_PATH/bin:$PATH"

# Install system libraries yang dibutuhkan oleh OpenCV
RUN apt-get update && apt-get install -y --no-install-recommends \
    libgl1-mesa-glx \
    libglib2.0-0 \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Pindah ke direktori kerja sementara
WORKDIR /deps

# Salin requirements.txt dan install dependensi ke dalam venv
COPY script/requirements.txt .
RUN pip install --no-cache-dir \
    --extra-index-url https://download.pytorch.org/whl/cpu \
    -r requirements.txt && \
    pip cache purge

# ===================================================================
# ⚙️ STAGE 2: GO BUILD (BUILDER LAYER)
#
# Tujuan: Mengkompilasi aplikasi Go menjadi satu file binary statis
# yang kecil dan efisien.
# ===================================================================
FROM golang:1.25-bookworm AS go-builder

WORKDIR /app

# Copy go mod files terlebih dahulu untuk memanfaatkan caching Docker
COPY go.mod go.sum ./
RUN go mod download

# Copy sisa source code dan build binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /prima_be .

# ===================================================================
# 🚀 STAGE 3: FINAL IMAGE (LIGHTWEIGHT & SECURE)
#
# Tujuan: Menggabungkan hasil build (Go binary & Python venv)
# ke dalam base image yang paling ringan.
# ===================================================================
FROM debian:bookworm-slim

# Install HANYA runtime dependencies yang mutlak diperlukan
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 \
    libgl1-mesa-glx \
    libglib2.0-0 \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# 1. Salin Python virtual environment yang sudah jadi dari stage python-base
ENV VENV_PATH=/opt/venv
COPY --from=python-base $VENV_PATH $VENV_PATH

# 2. Salin Go binary yang sudah di-compile dari stage go-builder
COPY --from=go-builder /prima_be .

# 3. Salin file proyek yang dibutuhkan saat runtime
COPY script/ ./script/
COPY model/ ./model/

# Buat direktori yang akan digunakan oleh aplikasi
RUN mkdir -p /app/processed_images
RUN mkdir -p /app/uploads

# (Opsional tapi direkomendasikan) Jalankan sebagai user non-root untuk keamanan
# RUN useradd --create-home --shell /bin/bash appuser
# USER appuser

EXPOSE 8080

# Atur PATH agar shell bisa menemukan Python dari venv kita
ENV PATH="$VENV_PATH/bin:$PATH"

# Jalankan aplikasi Go
CMD ["./prima_be"]