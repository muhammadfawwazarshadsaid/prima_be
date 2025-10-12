# ============================================================
# 🧩 STAGE 1: PYTHON DEPENDENCIES (HEAVY LAYER)
# ============================================================
FROM python:3.11-bookworm AS python-base
WORKDIR /deps

# Install runtime dependencies for OpenCV and Ultralytics
RUN apt-get update && apt-get install -y --no-install-recommends \
    libgl1 \
    libglib2.0-0 \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements and install (with torch CPU wheels)
COPY script/requirements.txt .
RUN pip install --no-cache-dir \
    --extra-index-url https://download.pytorch.org/whl/cpu \
    -r requirements.txt

# ============================================================
# ⚙️ STAGE 2: GO BUILD (FAST & LIGHT)
# ============================================================
FROM golang:1.25-bookworm AS go-builder
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# ============================================================
# 🚀 STAGE 3: FINAL IMAGE (LIGHTWEIGHT)
# ============================================================
FROM debian:bookworm-slim

# Install minimal runtime for Python & OpenCV libs
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 \
    python3-pip \
    libgl1-mesa-glx \
    libglib2.0-0 \
    && rm -rf /var/lib/apt/lists/*


WORKDIR /root

# Copy compiled Go binary
COPY --from=go-builder /app/main .

# Copy Python dependencies & environment
COPY --from=python-base /usr/local /usr/local

# Copy project files
COPY script/ /root/script/
COPY model/ /root/model/

EXPOSE 8080
CMD ["./main"]
