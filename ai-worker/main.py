import time
import logging
from config import WorkerConfig

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] [AI-WORKER] %(message)s")

def start_worker():
    logging.info("Starting Galeriku AI Worker subsystem...")
    logging.info(f"Target RabbitMQ: {WorkerConfig.RABBITMQ_HOST}:{WorkerConfig.RABBITMQ_PORT}")
    logging.info(f"Target Qdrant: {WorkerConfig.QDRANT_HOST}:{WorkerConfig.QDRANT_PORT}")
    logging.info(f"Target S3: {WorkerConfig.S3_ENDPOINT}")
    logging.info("AI Worker standing by for image processing queues...")

if __name__ == "__main__":
    start_worker()
