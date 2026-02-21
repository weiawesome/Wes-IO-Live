#!/bin/bash
mc alias set minio http://localhost:9000 minioadmin minioadmin

mc mb --ignore-existing minio/users
mc mb --ignore-existing minio/users-processed
mc mb --ignore-existing minio/vod

mc anonymous set download minio/users
mc anonymous set public minio/users-processed
mc anonymous set download minio/vod

# Remove existing overlapping or problematic event notifications first to avoid errors.
mc event remove minio/users --force

mc event add minio/users arn:minio:sqs::PRIMARY:kafka \
  --event put \
  --suffix .jpg

# Enable bucket lifecycle rule: objects with tag delete-pending are deleted after 7 days
mc ilm import minio/users <<EOF
{
  "Rules": [
    {
      "ID": "delete-pending-7d",
      "Filter": {
        "Tag": {
          "lifecycle": "delete-pending"
        }
      },
      "Status": "Enabled",
      "Expiration": {
        "Days": 7
      }
    }
  ]
}
EOF

mc ilm import minio/users-processed <<EOF
{
  "Rules": [
    {
      "ID": "delete-pending-7d",
      "Filter": {
        "Tag": {
          "lifecycle": "delete-pending"
        }
      },
      "Status": "Enabled",
      "Expiration": {
        "Days": 7
      }
    }
  ]
}
EOF