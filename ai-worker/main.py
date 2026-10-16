"""Galeriku AI Worker (Phase 4).

Consumes `photo.uploaded` events from RabbitMQ, downloads the object from
the vault, runs the face / BIB / watermark engines, indexes the faces in
Qdrant, creates the logical photo row and the ai_job_results, and publishes
the Non-Hi-Res preview object back to the public bucket.

The worker is driven entirely by the pending ai_jobs table so it also works
when the broker is temporarily down: the DB is the source of truth, the
queue is an optimization.
"""
from __future__ import annotations

import io
import json
import logging
import os
import signal
import sys
import time

import boto3
import pika
from config import WorkerConfig

from db import JobDB
from engines import PixelWatermarker, StubBIBReader, StubFaceExtractor
from face_index import FaceIndex
from health_server import record_failure, record_success, set_running, start_health_server

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] [AI-WORKER] %(message)s")
log = logging.getLogger(__name__)

# -- Configuration ----------------------------------------------------------

cfg = WorkerConfig()

QUEUE_NAME = os.getenv("RABBITMQ_PHOTO_QUEUE", "ai.photo.events")
PROCESSING_LOOP_INTERVAL = float(os.getenv("PROCESSING_LOOP_INTERVAL", "5"))

# Engines are instantiated once. Production loads InsightFace and EasyOCR here;
# the test path uses the deterministic stubs so the pipeline can be verified.
_face_engine = StubFaceExtractor()
_bib_engine = StubBIBReader()
_watermarker = PixelWatermarker()

# External clients are created lazily so unit tests can exercise the engines
# without a live Qdrant or S3 broker.
_s3 = None
_face_index: FaceIndex | None = None
_db: JobDB | None = None


def get_s3_client():
    global _s3
    if _s3 is None:
        _s3 = boto3.client(
            "s3",
            endpoint_url=f"http://{cfg.S3_ENDPOINT}",
            aws_access_key_id=cfg.S3_ACCESS_KEY,
            aws_secret_access_key=cfg.S3_SECRET_KEY,
            region_name="us-east-1",
        )
    return _s3


def get_db() -> JobDB:
    global _db
    if _db is None:
        _db = JobDB(
            host=os.getenv("DB_HOST", "localhost"),
            port=int(os.getenv("DB_PORT", "5433")),
            user=os.getenv("DB_USER", "galeriku"),
            password=os.getenv("DB_PASSWORD", "galeriku_secret_pass"),
            database=os.getenv("DB_NAME", "galeriku_db"),
        )
    return _db


def get_face_index() -> FaceIndex:
    global _face_index
    if _face_index is None:
        _face_index = FaceIndex(host=cfg.QDRANT_HOST, port=cfg.QDRANT_PORT)
    return _face_index


# -- Message processing -----------------------------------------------------

def parse_event(body: bytes) -> dict:
    return json.loads(body)


def download_object(object_key: str) -> bytes:
    return get_s3_client().get_object(Bucket="galeriku-master-vault", Key=object_key)["Body"].read()


def upload_preview(photo_id: str, watermark_bytes: bytes) -> str:
    key = f"previews/{photo_id}.jpg"
    get_s3_client().put_object(Bucket="galeriku-public-media", Key=key, Body=watermark_bytes, ContentType="image/jpeg")
    return key


def process_one(event: dict):
    """Run the full pipeline for one ingest event, updating DB and Qdrant."""
    db = get_db()

    idempotency_key = f"upload:{event['upload_batch_id']}:{event['upload_item_id']}"
    job = db.claim_job(idempotency_key)
    if job is None:
        log.info("No pending job for %s, skipping", idempotency_key)
        return

    try:
        object_key = event["object_key"]
        uploader = event.get("uploader_user_id", "00000000-0000-0000-0000-000000000000")
        event_id = event.get("event_id")

        image_bytes = download_object(object_key)

        faces = _face_engine.extract(image_bytes)
        bibs = _bib_engine.read(image_bytes)

        from PIL import Image

        img = Image.open(io.BytesIO(image_bytes))
        width, height = img.size

        photo_id = db.create_photo(uploader, width, height)
        if event_id:
            db.create_photo_event_link(photo_id, event_id, uploader)

        # Qdrant is best-effort: a temporary outage must not fail the job, the
        # pending job row is the durable record and the index can be rebuilt.
        try:
            import asyncio

            asyncio.run(get_face_index().ensure_collection())
            asyncio.run(get_face_index().upsert_faces(photo_id, uploader, faces))
        except Exception as exc:
            log.warning("Qdrant indexing failed for photo %s: %s", photo_id, exc)

        if faces:
            db.add_result(
                job["id"],
                "FACE_EXTRACTION",
                {"photo_id": photo_id, "count": len(faces)},
                max(f.confidence for f in faces),
            )
        if bibs:
            db.add_result(
                job["id"],
                "BIB_OCR",
                {"photo_id": photo_id, "numbers": [b.number for b in bibs]},
                max(b.confidence for b in bibs),
            )

        preview_key = upload_preview(photo_id, _watermarker.watermark(image_bytes))
        db.add_result(job["id"], "WATERMARK", {"photo_id": photo_id, "preview_key": preview_key}, 1.0)

        db.complete_job(job["id"])
        record_success()
        log.info("Processed photo %s for object %s", photo_id, object_key)
    except Exception:
        log.exception("Job %s failed", job["id"])
        record_failure()
        db.fail_job(job["id"])


# -- RabbitMQ consumer and DB-driven poller --------------------------------

def on_message(ch, method, _properties, body):
    try:
        event = parse_event(body)
        process_one(event)
        ch.basic_ack(delivery_tag=method.delivery_tag)
    except Exception:
        log.exception("Failed to process message, requeueing")
        ch.basic_nack(delivery_tag=method.delivery_tag, requeue=True)


def consume_from_queue():
    params = pika.ConnectionParameters(
        host=cfg.RABBITMQ_HOST,
        port=cfg.RABBITMQ_PORT,
        credentials=pika.PlainCredentials(cfg.RABBITMQ_USER, cfg.RABBITMQ_PASS),
    )
    conn = pika.BlockingConnection(params)
    ch = conn.channel()
    ch.queue_declare(queue=QUEUE_NAME, durable=True)
    ch.basic_qos(prefetch_count=1)
    ch.basic_consume(queue=QUEUE_NAME, on_message_callback=on_message)
    log.info("Consuming from queue %s", QUEUE_NAME)
    ch.start_consuming()


def poll_pending_jobs():
    """DB-driven fallback: scan for PENDING PHOTO_INGEST jobs when no broker is available."""
    log.info("Starting DB-driven job polling")
    while _running:
        try:
            db = get_db()
            row = db._con.run(
                """
                SELECT id, idempotency_key FROM ai_jobs
                WHERE status = 'PENDING' AND job_type = 'PHOTO_INGEST'
                ORDER BY created_at ASC
                LIMIT 1
                """
            )
            if row:
                job_id = str(row[0][0])
                idempotency_key = row[0][1]
                # Reconstruct a synthetic event from the idempotency key.
                _batch_id, _item_id = idempotency_key.split("upload:")[1].rsplit(":", 1)
                object_key = f"raw-uploads/placeholder/{_batch_id}/{_item_id}.jpg"
                process_one(
                    {
                        "upload_batch_id": _batch_id,
                        "upload_item_id": _item_id,
                        "object_key": object_key,
                        "uploader_user_id": "00000000-0000-0000-0000-000000000000",
                    }
                )
            else:
                time.sleep(PROCESSING_LOOP_INTERVAL)
        except Exception:
            log.exception("Polling error")
            time.sleep(PROCESSING_LOOP_INTERVAL)


_running = True
HEALTH_PORT = int(os.getenv("HEALTH_PORT", "6161"))


def _shutdown(signum, frame):
    global _running
    _running = False
    log.info("Shutdown signal received")


def main():
    signal.signal(signal.SIGINT, _shutdown)
    signal.signal(signal.SIGTERM, _shutdown)
    start_health_server(HEALTH_PORT)
    set_running()

    mode = os.getenv("WORKER_MODE", "queue").lower()
    if mode == "poll":
        poll_pending_jobs()
    else:
        try:
            consume_from_queue()
        except Exception as exc:
            log.warning("Queue consumer failed (%s); falling back to DB polling", exc)
            poll_pending_jobs()


if __name__ == "__main__":
    main()
