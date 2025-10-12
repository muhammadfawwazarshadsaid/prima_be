# ============================================================
# 🏗️ STAGE 1 — Builder (Go + Python deps)
# ============================================================
FROM golang:1.25-bookworm AS builder

WORKDIR /app

# ------------------------------------------------------------
# 1️⃣ Go dependencies
# ------------------------------------------------------------
COPY go.mod go.sum ./
RUN go mod download

# Copy seluruh source code Go
COPY . .

# Build binary Go
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# ------------------------------------------------------------
# 2️⃣ Install Python & pip
# ------------------------------------------------------------
RUN apt-get update && apt-get install -y python3 python3-pip

# Copy Python script dan requirements
COPY script/ /app/script/

# ------------------------------------------------------------
# 3️⃣ Install Python dependencies (CPU only, cached)
#    Use buildkit mount cache for faster rebuilds
# ------------------------------------------------------------
RUN --mount=type=cache,target=/root/.cache/pip \
    pip3 install --no-cache-dir \
    torch==2.8.0+cpu \
    torchvision==0.23.0+cpu \
    torchaudio==2.8.0+cpu \
    ultralytics==8.3.212 \
    scikit-image \
    jinja2 \
    --extra-index-url https://download.pytorch.org/whl/cpu

# ------------------------------------------------------------
# 4️⃣ Copy model dan siapkan folder kosong processed_images
# ------------------------------------------------------------
COPY model/ /app/model/
RUN mkdir -p /app/processed_images

# ============================================================
# 🚀 STAGE 2 — Runtime (Alpine ringan)
# ============================================================
FROM alpine:latest
WORKDIR /root/

# Copy binary Go
COPY --from=builder /app/main .

# Copy script & model
COPY --from=builder /app/script/ /root/script/
COPY --from=builder /app/model/ /root/model/

# Copy folder kosong processed_images (supaya gak error)
COPY --from=builder /app/processed_images /root/processed_images

# Copy env file
COPY .env .

EXPOSE 8080
CMD ["./main"]
