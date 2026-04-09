import json
import os
import boto3

_endpoint = os.environ.get("EMULATOR_ENDPOINT", "http://host.docker.internal:4566")
_region = "us-east-1"
_ddb = boto3.resource(
    "dynamodb",
    endpoint_url=_endpoint,
    region_name=_region,
    aws_access_key_id="test",
    aws_secret_access_key="test",
)
_table = _ddb.Table(os.environ.get("TABLE_NAME", "cdk-jobs-table-fixture"))


def handler(event, _context):
    processed = 0

    for record in event.get("Records", []):
        body = json.loads(record["body"])
        _table.put_item(
            Item={
                "id": body["id"],
                "status": "processed",
            }
        )
        processed += 1

    return {"processed": processed}
