#!/bin/sh
set -eu

curl --fail-with-body \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{"course_id":"sql-201","learner_id":"learner-7","asset_name":"query-plan.pdf","content_type":"application/pdf","bytes":2048,"due_at":"2030-08-14T10:00:00Z"}' \
  http://localhost:8080/uploads
