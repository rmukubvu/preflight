import json
import os
import uuid

import boto3


_sqs = boto3.client("sqs")


def handler(event, _context):
    payload = {}
    body = event.get("body")
    if body:
        payload = json.loads(body)

    job_id = payload.get("id") or str(uuid.uuid4())

    _sqs.send_message(
        QueueUrl=os.environ["QUEUE_URL"],
        MessageBody=json.dumps({"id": job_id}),
    )

    return {
        "statusCode": 202,
        "headers": {"content-type": "application/json"},
        "body": json.dumps({"id": job_id, "status": "queued"}),
    }
