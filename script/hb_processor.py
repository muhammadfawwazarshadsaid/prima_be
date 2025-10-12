# script/hb_processor.py

import sys
import os
import json
import numpy as np
import torch
from torchvision import transforms
import skimage.io as skio
from ultralytics import YOLO

# =====================================================
# 1) Definisi Model (Harus sama persis dengan saat training)
# =====================================================
class SimpleCNN(torch.nn.Module):
    def __init__(self):
        super(SimpleCNN, self).__init__()
        self.conv1 = torch.nn.Conv2d(3, 32, 3, 1, 1)
        self.conv2 = torch.nn.Conv2d(32, 64, 3, 1, 1)
        self.pool = torch.nn.MaxPool2d(2, 2)
        # Ukuran input ke fc1 tergantung pada resolusi gambar (setelah pooling)
        # Jika gambar input 64x64, setelah 2x pooling menjadi 16x16.
        # Jadi: 64 channel * 16 * 16
        self.fc1 = torch.nn.Linear(64 * 16 * 16, 128)
        self.fc2 = torch.nn.Linear(128, 1)

    def forward(self, x):
        x = self.pool(torch.relu(self.conv1(x)))
        x = self.pool(torch.relu(self.conv2(x)))
        x = x.view(x.size(0), -1)
        x = torch.relu(self.fc1(x))
        return self.fc2(x)

# =====================================================
# 2) Fungsi Helper dan Proses Utama
# =====================================================
def predict_hb_single(image_array, model, transform, device):
    """Prediksi Hb dari satu gambar crop."""
    try:
        # Konversi array numpy ke tensor
        img = transform(image_array).unsqueeze(0).to(device)
        with torch.no_grad():
            out = model(img)
        # Asumsi model mengeluarkan g/L, konversi ke g/dL
        return out.item() / 10.0
    except Exception as e:
        # Mengembalikan None jika ada error, misal gambar crop tidak valid
        return None

def main(source_image_path):
    """Fungsi utama untuk menjalankan seluruh proses AI."""
    # --- Konfigurasi Path (disesuaikan dengan struktur folder proyek) ---
    script_dir = os.path.dirname(__file__)
    YOLO_MODEL_PATH = os.path.join(script_dir, '..', 'model', 'best.pt')
    NAIL_HB_MODEL_PATH = os.path.join(script_dir, '..', 'model', 'nail_only_model.pth')
    
    # Direktori untuk menyimpan hasil, relatif terhadap folder utama proyek
    output_dir = os.path.join(script_dir, '..', 'processed_images')
    os.makedirs(output_dir, exist_ok=True)

    # --- Inisialisasi Model ---
    device = torch.device("cpu") # Paksa penggunaan CPU untuk kompatibilitas server
    transform = transforms.Compose([
        transforms.ToTensor(),
        transforms.Resize((64, 64), antialias=True) # antialias=True untuk kualitas lebih baik
    ])

    yolo_model = YOLO(YOLO_MODEL_PATH)
    nail_model = SimpleCNN().to(device)
    nail_model.load_state_dict(torch.load(NAIL_HB_MODEL_PATH, map_location=device))
    nail_model.eval()

    # --- Jalankan Deteksi YOLO ---
     yolo_results = yolo_model.predict(
        source=source_image_path,
        save=True,
        save_txt=True,
        project=output_dir,
        name="detection_result",
        exist_ok=True,
        conf=0.3,
        verbose=False, 
    )
    
    # Dapatkan path dari hasil YOLO
    latest_run_dir = yolo_results[0].save_dir
    base_filename = os.path.basename(source_image_path)
    filename_without_ext = os.path.splitext(base_filename)[0]
    label_path = os.path.join(latest_run_dir, 'labels', f"{filename_without_ext}.txt")
    bounded_box_image_path = os.path.join(latest_run_dir, base_filename)


    # --- Proses Hasil Deteksi & Prediksi Hb ---
    nail_crops = []
    try:
        original_img = skio.imread(source_image_path)
        H, W, _ = original_img.shape
    except Exception:
        # Jika gambar tidak bisa dibaca
        print(json.dumps({"error": "Failed to read source image file."}))
        sys.exit(1)


    if os.path.exists(label_path):
        with open(label_path, 'r') as f:
            for line in f:
                parts = list(map(float, line.strip().split()))
                cls, xc, yc, bw, bh = int(parts[0]), parts[1], parts[2], parts[3], parts[4]

                x1 = int((xc - bw / 2) * W)
                y1 = int((yc - bh / 2) * H)
                x2 = int((xc + bw / 2) * W)
                y2 = int((yc + bh / 2) * H)

                # 0 adalah kelas untuk 'nail' (sesuaikan dengan nama kelas di training YOLO Anda)
                if cls == 0:
                    nail_crops.append(original_img[y1:y2, x1:x2])

    # Lakukan prediksi hanya pada crop yang valid
    nail_hb_predictions = [pred for c in nail_crops if (pred := predict_hb_single(c, nail_model, transform, device)) is not None]

    # --- Siapkan Output JSON untuk Go ---
    final_output = {
        "nailBedResults": [],
        "conjunctivaResults": [], # Kosongkan jika model belum ada
        "boundedBoxImagePath": bounded_box_image_path,
        "detectionSuccess": False
    }

    if nail_hb_predictions:
        avg_hb = np.mean(nail_hb_predictions)
        std_dev = np.std(nail_hb_predictions) if len(nail_hb_predictions) > 1 else 0
        confidence = max(0, 100 - std_dev * 15) # Logika confidence sederhana
        
        final_output["detectionSuccess"] = True
        final_output["nailBedResults"].append({
            "objectType": "nail",
            "confidence": round(confidence, 1),
            "hbValue": round(avg_hb, 1)
        })

    # Cetak JSON ke stdout agar bisa ditangkap oleh Go
    print(json.dumps(final_output))

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print(json.dumps({"error": "No image path provided to Python script."}))
        sys.exit(1)
    
    image_path_arg = sys.argv[1]
    
    try:
        main(image_path_arg)
    except Exception as e:
        # Jika ada error tak terduga, kembalikan JSON error yang jelas
        error_message = {"error": f"An unexpected error occurred in Python script: {str(e)}"}
        print(json.dumps(error_message))
        sys.exit(1)