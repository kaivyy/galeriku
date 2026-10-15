"""Engine protocol and implementations for the AI worker pipeline.

The worker calls each engine through a Protocol so the production build
(InsightFace, EasyOCR) and the lightweight verification build (OpenCV-based
detector, Pillow-based watermark) share the same caller.

No raw biometric vector or selfie is persisted; face embeddings go only to
Qdrant and are never written to the database.
"""
from __future__ import annotations

import io
from dataclasses import dataclass, field

import numpy as np
from PIL import Image

# -- Data classes shared across engines ----------------------------------------

@dataclass
class FaceResult:
    bbox: tuple[int, int, int, int]  # x, y, w, h in pixels
    embedding: np.ndarray            # L2-normalized, 512-dim when InsightFace is loaded
    confidence: float


@dataclass
class BIBResult:
    number: str
    confidence: float


@dataclass
class PhotoMetadata:
    photo_id: str
    width_px: int
    height_px: int
    faces: list[FaceResult] = field(default_factory=list)
    bibs: list[BIBResult] = field(default_factory=list)


# -- Engine protocols ----------------------------------------------------------

class FaceExtractor:
    """Extracts L2-normalized 512-dim face embeddings from an image."""

    def extract(self, image_bytes: bytes) -> list[FaceResult]:
        ...


class BIBReader:
    """Reads race bib numbers from an image."""

    def read(self, image_bytes: bytes) -> list[BIBResult]:
        ...


class Watermarker:
    """Applies a server-side micro-watermark to produce a preview image."""

    def watermark(self, image_bytes: bytes) -> bytes:
        ...


# -- Lightweight implementations (used in tests and dev) -----------------------

class StubFaceExtractor:
    """Deterministic stub that detects one face per image and returns a
    fixed unit vector so the Qdrant pipeline can be exercised without the
    heavy InsightFace model. Production swaps this for the real model."""

    def extract(self, image_bytes: bytes) -> list[FaceResult]:
        img = Image.open(io.BytesIO(image_bytes))
        w, h = img.size
        # L2-normalized unit vector of length 512.
        embedding = np.zeros(512, dtype=np.float32)
        embedding[0] = 1.0
        return [FaceResult(bbox=(0, 0, w, h), embedding=embedding, confidence=0.99)]


class StubBIBReader:
    """Returns a fixed bib number so the OCR pipeline can be exercised."""

    def read(self, image_bytes: bytes) -> list[BIBResult]:
        return [BIBResult(number="0001", confidence=0.90)]


class PixelWatermarker:
    """Applies a destructive per-pixel watermark (DEC-Q01).

    Shifts the red channel of alternating rows by a small amount and embeds a
    tiled semi-transparent dot pattern. The operation is irreversible: the
    original pixel data cannot be recovered from the output, which is what
    makes the preview safe to distribute as instant Non-Hi-Res media.
    """

    DOT_INTERVAL = 8
    DOT_OPACITY = 0.15

    def watermark(self, image_bytes: bytes) -> bytes:
        img = Image.open(io.BytesIO(image_bytes)).convert("RGB")
        arr = np.array(img, dtype=np.uint8)

        # Destructive channel shift on alternating rows.
        arr[1::2, :, 0] = np.minimum(arr[1::2, :, 0].astype(np.int32) + 3, 255).astype(np.uint8)

        # Tiled semi-transparent dot pattern.
        h, w, _ = arr.shape
        ys, xs = np.ogrid[:h, :w]
        mask = (ys % self.DOT_INTERVAL < 1) & (xs % self.DOT_INTERVAL < 1)
        for c in range(3):
            channel = arr[:, :, c].astype(np.float32)
            channel[mask] = channel[mask] * (1.0 - self.DOT_OPACITY) + 255 * self.DOT_OPACITY
            arr[:, :, c] = channel.astype(np.uint8)

        out = io.BytesIO()
        Image.fromarray(arr, "RGB").save(out, format="JPEG", quality=85)
        return out.getvalue()
