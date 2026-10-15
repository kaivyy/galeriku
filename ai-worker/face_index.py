"""Qdrant vector indexing for face embeddings (Task 4.3).

Payload carries only {photo_id, subject_id, bbox}. The embedding itself lives
in the vector, per the UU PDP No. 27/2022 design decision: no raw biometric
data in PostgreSQL, and no biometric payload in the Qdrant metadata either.
"""
from __future__ import annotations

import logging

from qdrant_client import AsyncQdrantClient
from qdrant_client import models as qm

log = logging.getLogger(__name__)

FACE_COLLECTION = "galeriku_faces"
EMBEDDING_DIM = 512
COSINE = qm.Distance.COSINE


class FaceIndex:
    def __init__(self, host: str, port: int):
        self._client = AsyncQdrantClient(host=host, port=port)

    async def ensure_collection(self):
        """Creates the face collection when missing; idempotent otherwise."""
        cols = await self._client.get_collections()
        if any(c.name == FACE_COLLECTION for c in cols.collections):
            return
        await self._client.create_collection(
            collection_name=FACE_COLLECTION,
            vectors_config=qm.VectorParams(size=EMBEDDING_DIM, distance=COSINE),
        )
        log.info("Created Qdrant collection %s (%d-dim, cosine)", FACE_COLLECTION, EMBEDDING_DIM)

    async def upsert_faces(self, photo_id: str, subject_id: str, faces) -> int:
        """Index every face on one photo and return how many points were written."""
        if not faces:
            return 0
        points = [
            qm.PointStruct(
                id=_deterministic_point_id(photo_id, i),
                vector=f.embedding.tolist(),
                payload={
                    "photo_id": photo_id,
                    "subject_id": subject_id,
                    "bbox": list(f.bbox),
                },
            )
            for i, f in enumerate(faces)
        ]
        await self._client.upsert(collection_name=FACE_COLLECTION, points=points, wait=True)
        log.info("Indexed %d face(s) for photo %s", len(points), photo_id)
        return len(points)


def _deterministic_point_id(photo_id: str, index: int):
    """Derives a stable UUID from photo_id and the face index, so reprocessing
    the same photo updates its points instead of duplicating them."""
    import uuid

    return str(uuid.uuid5(uuid.NAMESPACE_URL, f"galeriku:{photo_id}:face:{index}"))
