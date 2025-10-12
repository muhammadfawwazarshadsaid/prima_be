# Menggunakan base image Debian (Bookworm) dengan Go versi 1.25 sebagai image final
FROM golang:1.25-bookworm

WORKDIR /app

# ====================================================================
# Optimasi Cache: Install Dependensi Python (yang paling lambat)
# ====================================================================
# ## PERBAIKAN DI SINI: Menambahkan libgl1-mesa-glx ##
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 \
    python3-pip \
    libgl1-mesa-glx \
    && rm -rf /var/lib/apt/lists/*

COPY script/requirements.txt /app/script/requirements.txt
RUN pip3 install --no-cache-dir \
    --extra-index-url https://download.pytorch.org/whl/cpu \
    -r /app/script/requirements.txt \
    --break-system-packages

# ====================================================================
# Optimasi Cache: Install Dependensi Go
# ====================================================================
COPY go.mod go.sum ./
RUN go mod download

# ====================================================================
# Copy Seluruh Kode Aplikasi dan Build
# ====================================================================
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# Expose port dan jalankan aplikasi
EXPOSE 8080
CMD ["./main"]