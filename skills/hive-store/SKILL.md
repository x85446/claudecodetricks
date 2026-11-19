---
name: hive-store
description: Check Gravhl Hive Mind automation access and read or update Hive Mind documents with the shared automation account. Use for document-store requests that identify a site, collection, or document.
---

# Hive Mind document store

Use `scripts/hive-store.py` from this skill's folder, or `hive-store.py` at the repository root when using a checkout. In the commands below, use the full path to the script if it is outside the working directory.

The Hive Mind key button in [Management](https://mgmt.gravhl.com/?tab=mgmt) supplies `HIVE_URL`, `HIVE_SERVICE_KEY`, `CF_ACCESS_CLIENT_ID`, and `CF_ACCESS_CLIENT_SECRET`. Use the configured environment and keep credential values out of responses and command arguments. The hosted API origin is `https://hivemind-mgmt.gravhl.com`. Run `python3 hive-store.py me` to check access as the shared automation account.

- Run `python3 hive-store.py list map assignments` to list a collection, or `python3 hive-store.py get map ASSIGNMENTS_PATH` to read a specific document. Select the site and path from the request or an API response.
- For a requested edit, read the document and use `python3 hive-store.py update SITE DOCUMENT --file patch.json --if-version VERSION` with the returned version. If the version changed, read the document again and review the edit before retrying. Confirm the change from the response.
- Use the Hive Cards skill and a personal card token for personal or teammate queues and card completions.
