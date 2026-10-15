"""Live verification for the Phase 4 AI worker pipeline.

Run from the ai-worker directory with the venv:
    .venv/bin/python -m pytest test_worker.py -v
or directly:
    .venv/bin/python test_worker.py

The test suite is designed to run against live PostgreSQL, MinIO, Qdrant, and
RabbitMQ containers; when a dependency is unavailable the affected test skips
itself so the remaining evidence still stands.
"""
import io
import json
import os
import sys
import uuid

import numpy as np
from PIL import Image

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from engines import PixelWatermarker, StubBIBReader, StubFaceExtractor  # noqa: E402
from db import JobDB  # noqa: E402

PASS = []
FAIL = []
SKIP = []


def check(name):
    def wrap(fn):
        try:
            fn()
            PASS.append(name)
            print(f"  PASS  {name}")
        except SkipTest as e:
            SKIP.append(name)
            print(f"  SKIP  {name}: {e}")
        except Exception as e:
            FAIL.append((name, repr(e)))
            print(f"  FAIL  {name}: {e!r}")
    return wrap


class SkipTest(Exception):
    pass


def live_db() -> JobDB:
    env = os.environ
    try:
        return JobDB(
            host=env.get("DB_HOST", "localhost"),
            port=int(env.get("DB_PORT", "5433")),
            user=env.get("DB_USER", "galeriku"),
            password=env.get("DB_PASSWORD", "galeriku_secret_pass"),
            database=env.get("DB_NAME", "galeriku_db"),
        )
    except Exception as e:
        raise SkipTest(f"no live postgres: {e}")


# 1. Face engine returns one 512-dim L2-normalized embedding per photo.
@check("face engine returns 512-dim L2-normalized embedding")
def _():
    img = Image.new("RGB", (640, 480), color=(128, 128, 128))
    buf = io.BytesIO()
    img.save(buf, format="JPEG")

    faces = StubFaceExtractor().extract(buf.getvalue())
    assert len(faces) == 1
    embedding = faces[0].embedding
    assert embedding.shape == (512,)
    norm = float(np.linalg.norm(embedding))
    assert abs(norm - 1.0) < 1e-5, f"expected L2 norm 1.0, got {norm}"
    assert 0.0 <= faces[0].confidence <= 1.0


# 2. BIB engine returns one numeric bib string per photo.
@check("bib engine returns numeric normalized bib")
def _():
    img = Image.new("RGB", (640, 480), color=(255, 255, 255))
    buf = io.BytesIO()
    img.save(buf, format="JPEG")

    bibs = StubBIBReader().read(buf.getvalue())
    assert len(bibs) == 1
    assert bibs[0].number.isdigit(), f"bib must be numeric, got {bibs[0].number!r}"


# 3. Decoding back the watermarked bytes has two visible effects:
#    alternating rows are shifted in the red channel, and a dot grid appears.
@check("watermark is destructive and pixel-visible")
def _():
    img = Image.new("RGB", (64, 64), color=(200, 200, 200))
    buf = io.BytesIO()
    img.save(buf, format="JPEG")

    wm_bytes = PixelWatermarker().watermark(buf.getvalue())
    arr = np.array(Image.open(io.BytesIO(wm_bytes)))
    assert arr.shape[2] == 3

    # A flat original must come back non-flat: the destructive dot grid and row
    # shift introduce variance that no JPEG recompression of a flat input has.
    assert arr.std() > 0.5, f"expected pixel variance from the watermark, got std {arr.std()}"


# 4. The full pipeline against live services: create a real object in the vault,
#    insert a PENDING ai_job, run process_one, then assert the photo row, the
#    ai_job_results, the Qdrant point, and the preview object.
@check("pipeline end-to-end against live services")
def _():
    try:
        import boto3
    except ImportError:
        raise SkipTest("boto3 not installed")

    db = live_db()

    # Seed storage fixture so the photo_event_link (optional) can be skipped and
    # the raw object can live in the vault.
    import boto3

    s3 = boto3.client(
        "s3",
        endpoint_url="http://" + os.environ.get("S3_ENDPOINT", "localhost:9010"),
        aws_access_key_id=os.environ.get("MINIO_ROOT_USER", "galeriku_admin"),
        aws_secret_access_key=os.environ.get("MINIO_ROOT_PASSWORD", "galeriku_minio_secret"),
        region_name="us-east-1",
    )
    try:
        s3.head_bucket(Bucket="galeriku-master-vault")
    except Exception as e:
        raise SkipTest(f"MinIO not available: {e}")

    # Generate a deterministic face test image.
    img = Image.new("RGB", (320, 240), color=(150, 100, 100))
    buf = io.BytesIO()
    img.save(buf, format="JPEG")
    test_bytes = buf.getvalue()

    batch_id = str(uuid.uuid4())
    item_id = str(uuid.uuid4())
    uploader = str(uuid.uuid4())
    object_key = f"raw-uploads/test/{batch_id}/{item_id}.jpg"

    # Insert the object into MinIO.
    s3.put_object(Bucket="galeriku-master-vault", Key=object_key, Body=test_bytes, ContentType="image/jpeg")

    # Use a deterministic idempotency key so re-running the test works.
    test_token = str(uuid.uuid4())
    idempotency_key = f"upload:{batch_id}:{item_id}"
    db.run(
        """
        INSERT INTO ai_jobs (job_type, subject_id, status, idempotency_key, attempt_count)
        VALUES ('PHOTO_INGEST', NULL, 'PENDING', :key, 0)
        """,
        key=idempotency_key,
    )

    # Seed a resolved uploader: a real user row so photos.uploader_user_id FK holds.
    db.run(
        """
        INSERT INTO users (id, email_normalized, account_status)
        VALUES (:id, :email, 'ACTIVE')
        """,
        id=uuid.UUID(uploader),
        email=f"ai_worker_test_{test_token[:8]}@galeriku-test.invalid",
    )
    db.run(
        """
        INSERT INTO user_profiles (user_id, display_name, locale)
        VALUES (:id, 'AI Test', 'id-ID')
        """,
        id=uuid.UUID(uploader),
    )

    # Exercise the worker pipeline.
    import importlib

    main = importlib.import_module("main")

    event = {
        "upload_batch_id": batch_id,
        "upload_item_id": item_id,
        "object_key": object_key,
        "uploader_user_id": uploader,
    }
    main.process_one(event)

    # Assert: the job completed.
    status = db.one(
        "SELECT status::text FROM ai_jobs WHERE idempotency_key = :key", key=idempotency_key
    )[0]
    assert status == "COMPLETED", f"expected job COMPLETED, got {status}"

    # Assert: a photo row exists and is linked.
    photo_row = db.one(
        """
        SELECT p.id::text, p.width_px, p.height_px
        FROM photos p
        WHERE p.uploader_user_id = :uploader
        ORDER BY p.created_at DESC
        LIMIT 1
        """,
        uploader=uuid.UUID(uploader),
    )
    assert photo_row is not None, "expected a photos row after processing"
    photo_id, w, h = photo_row[0], photo_row[1], photo_row[2]
    assert (w, h) == (320, 240), f"expected 320x240, got {w}x{h}"

    # Assert: three ai_job_results rows (faces, BIB, watermark).
    types = db.one(
        """
        SELECT COUNT(*) FROM ai_job_results r
        JOIN ai_jobs j ON j.id = r.ai_job_id
        WHERE j.idempotency_key = :key
        """,
        key=idempotency_key,
    )[0]
    assert types == 3, f"expected 3 results (face, bib, watermark), got {types}"

    # Assert: the preview object exists in the public bucket.
    preview_key = f"previews/{photo_id}.jpg"
    s3.head_object(Bucket="galeriku-public-media", Key=preview_key)

    # Assert: a face point exists in Qdrant.
    from qdrant_client import QdrantClient

    qdrant = QdrantClient(host=os.environ.get("QDRANT_HOST", "localhost"), port=int(os.environ.get("QDRANT_PORT", "6333")))
    points = qdrant.scroll(
        collection_name="galeriku_faces",
        scroll_filter={"must": [{"key": "photo_id", "match": {"value": photo_id}}]},
        limit=10,
    )
    found = points[0]
    assert len(found) >= 1, "expected at least 1 face point in Qdrant"

    # Cleanup: remove test data.
    db.run(
        """
        DELETE FROM ai_job_results WHERE ai_job_id IN (SELECT id FROM ai_jobs WHERE idempotency_key = :key)
        """,
        key=idempotency_key,
    )
    db.run("DELETE FROM ai_jobs WHERE idempotency_key = :key", key=idempotency_key)
    db.run("DELETE FROM photos WHERE id = :pid", pid=uuid.UUID(photo_id))
    try:
        s3.delete_object(Bucket="galeriku-master-vault", Key=object_key)
        s3.delete_object(Bucket="galeriku-public-media", Key=preview_key)
    except Exception:
        pass
    db.run("DELETE FROM user_profiles WHERE user_id = :id", id=uuid.UUID(uploader))
    db.run("DELETE FROM users WHERE id = :id", id=uuid.UUID(uploader))
    db.close()


# 5. Consumer parses the RabbitMQ message shape exactly as published by Go.
@check("event parsing matches the Go publisher payload")
def _():
    from main import parse_event

    publisher_payload = {
        "event_type": "photo.uploaded",
        "object_key": "raw-uploads/x/y/z.jpg",
        "upload_batch_id": str(uuid.uuid4()),
        "upload_item_id": str(uuid.uuid4()),
        "storage_account_id": str(uuid.uuid4()),
        "actual_bytes": 1234,
        "occurred_at": "2026-10-16T00:00:00Z",
    }
    event = parse_event(json.dumps(publisher_payload).encode())
    assert event["object_key"] == publisher_payload["object_key"]
    assert event["upload_batch_id"] == publisher_payload["upload_batch_id"]
    assert event["upload_item_id"] == publisher_payload["upload_item_id"]


if __name__ == "__main__":
    print(f"\nPhase 4 verification: {len(PASS)} pass, {len(FAIL)} fail, {len(SKIP)} skipped")
    if FAIL:
        for name, err in FAIL:
            print(f"  FAILED: {name}: {err}")
        sys.exit(1)
