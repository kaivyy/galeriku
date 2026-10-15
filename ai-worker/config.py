import os
from dotenv import load_dotenv

load_dotenv()

class WorkerConfig:
    RABBITMQ_HOST = os.getenv("RABBITMQ_HOST", "localhost")
    RABBITMQ_PORT = int(os.getenv("RABBITMQ_PORT", "5672"))
    RABBITMQ_USER = os.getenv("RABBITMQ_USER", "guest")
    RABBITMQ_PASS = os.getenv("RABBITMQ_PASSWORD", "guest")
    
    QDRANT_HOST = os.getenv("QDRANT_HOST", "localhost")
    QDRANT_PORT = int(os.getenv("QDRANT_PORT", "6333"))
    
    S3_ENDPOINT = os.getenv("S3_ENDPOINT", "localhost:9000")
    S3_ACCESS_KEY = os.getenv("S3_ACCESS_KEY", "galeriku_admin")
    S3_SECRET_KEY = os.getenv("S3_SECRET_KEY", "galeriku_minio_secret")
    S3_USE_SSL = os.getenv("S3_USE_SSL", "false").lower() == "true"
