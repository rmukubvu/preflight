import json
import uuid

import boto3

_endpoint = "http://preflight-floci:4566"
_region = "us-east-1"
_queue_url = "http://preflight-floci:4566/000000000000/cdk-job-queue-fixture"
_sqs = boto3.client(
    "sqs",
    endpoint_url=_endpoint,
    region_name=_region,
    aws_access_key_id="test",
    aws_secret_access_key="test",
)


def handler(event, _context):
    payload = {}
    body = event.get("body")
    if body:
        payload = json.loads(body)

    job_id = payload.get("id") or str(uuid.uuid4())
    _sqs.send_message(
        QueueUrl=_queue_url,
        MessageBody=json.dumps({"id": job_id}),
    )

    return {
        "statusCode": 202,
        "headers": {"content-type": "application/json"},
        "body": json.dumps({"id": job_id, "status": "queued"}),
    }
