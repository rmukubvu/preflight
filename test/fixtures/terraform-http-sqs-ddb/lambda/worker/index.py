import json
import os

import boto3


_table = boto3.resource("dynamodb").Table(os.environ["TABLE_NAME"])


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
