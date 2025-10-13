# script/diary_analyzer.py

import sys
import os
import base64
import json
import requests
from moviepy.editor import VideoFileClip

# Script ini akan dipanggil dengan: python3 diary_analyzer.py <video_path> <api_key>

def crop_video_to_10s(video_path, output_path="trimmed_10s.mp4"):
    """Potong video menjadi 10 detik pertama."""
    try:
        with VideoFileClip(video_path) as clip:
            duration = min(clip.duration, 10)
            trimmed = clip.subclip(0, duration)
            # Simpan file sementara di direktori yang sama dengan aslinya
            temp_output = os.path.join(os.path.dirname(video_path), output_path)
            trimmed.write_videofile(temp_output, codec="libx264", audio_codec="aac", verbose=False, logger=None)
            return temp_output
    except Exception as e:
        # Jika gagal memotong, gunakan video asli dan log peringatan.
        sys.stderr.write(f"Warning: Gagal memotong video: {e}. Menggunakan video asli.\n")
        return video_path


def analyze_video(video_path, api_key):
    """Kirim video ke Gemini dan minta output JSON terstruktur."""
    gemini_url = "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:generateContent"

    with open(video_path, "rb") as f:
        video_b64 = base64.b64encode(f.read()).decode("utf-8")

    # Prompt yang lebih tangguh, meminta output JSON secara eksplisit
    prompt = (
        "Analisis video ini dan berikan ringkasan perkembangannya. "
        "Fokus pada interaksi, suara, dan gerakan bayi. "
        "Hasil harus dalam format JSON dengan kunci berikut: 'summary' (string). "
        "Isi 'summary' harus berupa satu paragraf naratif yang ringkas (maksimal 3 kalimat) menjelaskan momen dalam video. "
        "Contoh: {\"summary\": \"Di video ini, si kecil terlihat senang merespon saat diajak bermain cilukba dan beberapa kali meniru suara tawa orang tuanya. Gerakannya aktif dan ia tampak ceria.\"} "
        "Jangan tambahkan markdown atau karakter lain di luar JSON object."
    )

    payload = {
        "contents": [
            {
                "role": "user",
                "parts": [
                    {"text": prompt},
                    {"inline_data": {"mime_type": "video/mp4", "data": video_b64}}
                ]
            }
        ]
    }

    try:
        res = requests.post(f"{gemini_url}?key={api_key}", json=payload, timeout=90) # Timeout 90 detik
        res.raise_for_status() # Menghasilkan HTTPError untuk response buruk (4xx or 5xx)

        data = res.json()
        
        # Ekstrak bagian teks yang seharusnya berisi string JSON
        raw_text = data["candidates"][0]["content"]["parts"][0]["text"]
        
        # Bersihkan dari format markdown (```json ... ```) yang mungkin muncul
        cleaned_text = raw_text.strip().replace("```json", "").replace("```", "").strip()
        
        # Parse teks yang sudah bersih sebagai JSON
        result_json = json.loads(cleaned_text)
        return result_json

    except requests.exceptions.RequestException as e:
        return {"error": f"Error jaringan saat menghubungi Gemini API: {e}"}
    except (KeyError, IndexError) as e:
        return {"error": f"Format respons dari Gemini API tidak terduga: {e}. Respons: {res.text}"}
    except json.JSONDecodeError as e:
        return {"error": f"Gagal mem-parsing JSON dari respons Gemini: {e}. Teks mentah: '{cleaned_text}'"}
    except Exception as e:
        return {"error": f"Terjadi error yang tidak diketahui saat analisis video: {e}"}


def main():
    if len(sys.argv) < 3:
        print(json.dumps({"error": "Path video atau API key tidak disediakan."}))
        sys.exit(1)

    input_video_path = sys.argv[1]
    api_key = sys.argv[2]
    
    # Potong video terlebih dahulu
    trimmed_video_path = crop_video_to_10s(input_video_path)

    # Analisis video (yang mungkin sudah dipotong)
    result = analyze_video(trimmed_video_path, api_key)
    
    # Hapus file video yang sudah dipotong jika ada
    if trimmed_video_path != input_video_path and os.path.exists(trimmed_video_path):
        try:
            os.remove(trimmed_video_path)
        except OSError as e:
            sys.stderr.write(f"Warning: Tidak dapat menghapus file sementara {trimmed_video_path}: {e}\n")

    # Cetak hasil JSON akhir ke stdout
    print(json.dumps(result))

if __name__ == "__main__":
    main()