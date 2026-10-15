"""Database access for the AI worker: job claiming and result persistence.

Uses pure-Python pg8000 so the worker stays installable without build tools.
The native pg8000 Connection.run() returns a list of rows directly; this
module wraps it with helpers that match the caller's expectations.
"""
import json
import uuid

import pg8000.native


class JobDB:
    def __init__(self, host: str, port: int, user: str, password: str, database: str):
        self._con = pg8000.native.Connection(
            user=user, password=password, host=host, port=port, database=database
        )

    @property
    def conn(self):
        return self._con

    def close(self):
        self._con.close()

    def run(self, sql: str, **params):
        """Execute a statement and return the list of rows it returned."""
        return self._con.run(sql, **params)

    def one(self, sql: str, **params):
        """Execute and return the first row as a tuple, or None when no rows match."""
        rows = self._con.run(sql, **params)
        return rows[0] if rows else None

    def claim_job(self, idempotency_key: str) -> dict | None:
        """Atomically claim one PENDING job row.

        Uses FOR UPDATE SKIP LOCKED inside a transaction so competing workers
        never take the same job.
        """
        rows = self._con.run(
            """
            WITH picked AS (
                SELECT id FROM ai_jobs
                WHERE status = 'PENDING' AND idempotency_key = :key
                FOR UPDATE SKIP LOCKED
                LIMIT 1
            )
            UPDATE ai_jobs j
            SET status = 'PROCESSING', attempt_count = j.attempt_count + 1
            FROM picked
            WHERE j.id = picked.id
            RETURNING j.id
            """,
            key=idempotency_key,
        )
        if not rows:
            return None
        return {"id": str(rows[0][0])}

    def complete_job(self, job_id: str):
        self._con.run(
            "UPDATE ai_jobs SET status = 'COMPLETED', completed_at = CURRENT_TIMESTAMP WHERE id = :id",
            id=uuid.UUID(job_id),
        )

    def fail_job(self, job_id: str):
        self._con.run(
            "UPDATE ai_jobs SET status = 'FAILED', completed_at = CURRENT_TIMESTAMP WHERE id = :id",
            id=uuid.UUID(job_id),
        )

    def add_result(self, job_id: str, result_type: str, reference: dict, confidence: float | None):
        self._con.run(
            """
            INSERT INTO ai_job_results (ai_job_id, result_type, result_reference, confidence, status)
            VALUES (:job_id, :rtype, :ref::jsonb, :confidence, 'OK')
            """,
            job_id=uuid.UUID(job_id),
            rtype=result_type,
            ref=json.dumps(reference),
            confidence=confidence,
        )

    def create_photo(self, uploader_user_id: str, width: int, height: int) -> str:
        rows = self._con.run(
            """
            INSERT INTO photos (uploader_user_id, status, width_px, height_px, moderation_status)
            VALUES (:uploader, 'READY', :w, :h, 'PENDING_REVIEW')
            RETURNING id
            """,
            uploader=uuid.UUID(uploader_user_id),
            w=width,
            h=height,
        )
        return str(rows[0][0])

    def create_photo_event_link(self, photo_id: str, event_id: str, user_id: str):
        if event_id:
            self._con.run(
                """
                INSERT INTO photo_event_links (photo_id, event_id, linked_by)
                VALUES (:photo, :event, :user)
                """,
                photo=uuid.UUID(photo_id),
                event=uuid.UUID(event_id),
                user=uuid.UUID(user_id),
            )
